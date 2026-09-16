package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/storage"
)

// These tests exist because Router's dispatch table had none, and that is the
// whole reason the proxy shipped forwarding only "/api/*". Every request to a
// path outside that prefix was answered by the dashboard with a 200 and never
// reached the sniffer, so for most real APIs the product was a no-op.
//
// The backend hit counter is load-bearing. Asserting on the response body alone
// cannot tell "the control plane answered" apart from "proxied to a backend that
// 404s" — and that ambiguity is precisely how the bug survived review.

type harness struct {
	router *httptest.Server
	hits   *int64
	store  *storage.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	// storage.NewStore resolves its persist path from the home directory, and
	// the proxy auto-saves a baseline for the first JSON response it sees — so
	// without this the suite reads the developer's real ~/.driftwood and writes
	// test endpoints into it. Moving HOME to a temp dir makes each test start
	// from an empty store, which is also what makes the assertions below exact.
	t.Setenv("HOME", t.TempDir())

	var hits int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		// Echo an ACAO the way a real CORS-enabled backend would, so the
		// duplicate-header regression is reproducible rather than hypothetical.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"backend"}`))
	}))
	t.Cleanup(backend.Close)

	store := storage.NewStore(backend.URL, "8787")
	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	prx, err := proxy.NewProxyForTest(backend.URL, store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("NewProxyForTest: %v", err)
	}

	front := httptest.NewServer(NewServer(store, hub, prx, mockCtrl).Router())
	t.Cleanup(front.Close)

	return &harness{router: front, hits: &hits, store: store}
}

func (h *harness) do(t *testing.T, method, path string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, h.router.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", method, path, err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	// The front server's own redirect handling would follow the bare-namespace
	// redirect and hide it, so redirects are surfaced instead of followed.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *harness) hitCount() int64 { return atomic.LoadInt64(h.hits) }

// The regression Phase 1 exists for: an endpoint outside the control namespace
// reaches the target API and is recorded, whatever its prefix.
func TestAnyNonControlPathIsProxied(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/api/users", "/v1/users", "/graphql", "/orders", "/"} {
		before := h.hitCount()
		resp := h.do(t, http.MethodGet, path, nil)

		if got := h.hitCount(); got != before+1 {
			t.Errorf("GET %s: backend hits %d -> %d, want exactly one", path, before, got)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestProxiedRequestIsRecordedAsTraffic(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodGet, "/v1/orders", nil)

	var found bool
	for _, tr := range h.store.GetTraffics(100) {
		if tr.Path == "/v1/orders" {
			found = true
		}
	}
	if !found {
		t.Error("expected /v1/orders to be recorded as traffic; the sniffer never saw it")
	}
}

func TestProxiedRequestRecordsAnObservation(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodGet, "/v1/orders", nil)

	// Asserted through the real request path rather than by calling AddTraffic,
	// because the wiring is the point: AddTraffic existing and being correct is
	// worth nothing if nothing joins it to the endpoint's history. Until it did,
	// Versions grew only when a human saved a baseline, so the History view had
	// no data by construction and the stability trend had no series to draw.
	hist, ok := h.store.GetHistory("GET", "/v1/orders")
	if !ok {
		t.Fatal("proxied request produced no history entry")
	}
	if len(hist.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(hist.Observations))
	}
	obs := hist.Observations[0]
	if obs.StatusCode != http.StatusOK {
		t.Errorf("observation status = %d, want 200", obs.StatusCode)
	}
	if obs.Timestamp.IsZero() {
		t.Error("observation has no timestamp")
	}
	// The contract status is what the dashboard plots per observation. Its exact
	// value depends on the baseline that existed at the time, so this asserts it
	// was carried through rather than what it was.
	if obs.ContractStatus == "" {
		t.Error("observation recorded no contract status")
	}
	if hist.ObservationCount != 1 {
		t.Errorf("observation_count = %d, want 1", hist.ObservationCount)
	}
}

func TestControlAPIIsNotProxied(t *testing.T) {
	h := newHarness(t)
	before := h.hitCount()

	resp := h.do(t, http.MethodGet, proxy.ControlPrefix+"/api/traffic", nil)

	if got := h.hitCount(); got != before {
		t.Errorf("control API reached the target backend (%d -> %d)", before, got)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON from the control plane", ct)
	}
}

// The boundary is the trailing slash. "/_driftwoodfoo" is somebody's API path.
func TestNamespaceBoundary(t *testing.T) {
	h := newHarness(t)

	proxied := []string{"/_driftwoodfoo", "/_driftwoodish/users", "/_driftwoo"}
	for _, path := range proxied {
		before := h.hitCount()
		h.do(t, http.MethodGet, path, nil)
		if got := h.hitCount(); got != before+1 {
			t.Errorf("GET %s: expected proxy to backend, hits %d -> %d", path, before, got)
		}
	}

	before := h.hitCount()
	h.do(t, http.MethodGet, proxy.ControlPrefix+"/api/alerts", nil)
	if got := h.hitCount(); got != before {
		t.Errorf("control path leaked to the backend (%d -> %d)", before, got)
	}
}

// "./"-relative assets need a base, so the bare namespace must not serve the doc.
func TestBareNamespaceRedirectsToTrailingSlash(t *testing.T) {
	h := newHarness(t)
	before := h.hitCount()

	resp := h.do(t, http.MethodGet, proxy.ControlPrefix, nil)

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != proxy.ControlPrefix+"/" {
		t.Errorf("Location = %q, want %q", got, proxy.ControlPrefix+"/")
	}
	if got := h.hitCount(); got != before {
		t.Error("the redirect must not reach the backend")
	}
}

// A preflight is the backend's business, not ours. This used to be swallowed
// above the routing, so the backend never saw it — a CORS regression that curl
// cannot reveal because curl does not send preflights.
func TestPreflightForProxiedPathReachesBackend(t *testing.T) {
	h := newHarness(t)
	before := h.hitCount()

	h.do(t, http.MethodOptions, "/v1/users", map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "GET",
	})

	if got := h.hitCount(); got != before+1 {
		t.Errorf("OPTIONS was swallowed: backend hits %d -> %d, want one", before, got)
	}
}

func TestPreflightForControlAPIIsAnsweredLocally(t *testing.T) {
	h := newHarness(t)
	before := h.hitCount()

	resp := h.do(t, http.MethodOptions, proxy.ControlPrefix+"/api/traffic", map[string]string{
		"Origin": "http://localhost:8787",
	})

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := h.hitCount(); got != before {
		t.Error("control preflight must not reach the backend")
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8787" {
		t.Errorf("ACAO = %q, want the echoed allowed origin", got)
	}
}

// The duplicate-header regression. Router set ACAO and httputil copied the
// backend's on top with Add, emitting the header twice — which browsers reject
// outright. It was invisible while only "/api/*" was proxied and would have
// broken every request once the gate widened.
func TestProxiedResponseDoesNotDuplicateCORS(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, "/v1/users", map[string]string{
		"Origin": "http://localhost:8787",
	})

	got := resp.Header.Values("Access-Control-Allow-Origin")
	if len(got) != 1 {
		t.Fatalf("Access-Control-Allow-Origin sent %d times %v; browsers reject duplicates", len(got), got)
	}
	if got[0] != "*" {
		t.Errorf("ACAO = %q, want the backend's own value passed through untouched", got[0])
	}
}

func TestUnknownControlAPIEndpointIs404JSON(t *testing.T) {
	h := newHarness(t)
	before := h.hitCount()

	resp := h.do(t, http.MethodGet, proxy.ControlPrefix+"/api/nope", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q; a mistyped endpoint must not return HTML", ct)
	}
	if got := h.hitCount(); got != before {
		t.Error("an unknown control path must not be proxied")
	}
}

// The bare mock path used to pass the router's HasPrefix check but fail the
// proxy's, which required a trailing slash — so it silently became a real
// backend call.
func TestMockBarePathIsNotProxied(t *testing.T) {
	for _, path := range []string{proxy.MockPrefix, proxy.MockPrefix + "/users"} {
		h := newHarness(t)
		before := h.hitCount()

		resp := h.do(t, http.MethodGet, path, nil)

		if got := h.hitCount(); got != before {
			t.Errorf("GET %s: mock path reached the backend (%d -> %d)", path, before, got)
		}
		if resp.StatusCode == http.StatusBadGateway {
			t.Errorf("GET %s: got 502, the mock controller did not answer", path)
		}
	}
}

// web.ServeIndex resolves web/index.html relative to the process working
// directory, so this one test has to run from the module root.
func TestDashboardServedUnderNamespace(t *testing.T) {
	t.Chdir("../..")
	h := newHarness(t)
	before := h.hitCount()

	resp := h.do(t, http.MethodGet, proxy.ControlPrefix+"/", nil)
	body := readAll(t, resp)

	if got := h.hitCount(); got != before {
		t.Error("the dashboard was proxied to the backend")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, truncate(body))
	}
	if !strings.Contains(strings.ToLower(body), "<html") {
		t.Errorf("expected the dashboard document, got: %s", truncate(body))
	}
}

// The relative asset URLs are what make the namespace relocatable, and an
// absolute one would be proxied to the target API once the gate widened — the
// dashboard would 404 its own stylesheet while still returning 200.
func TestDashboardAssetsResolveUnderNamespace(t *testing.T) {
	t.Chdir("../..")
	// frontend-react/dist is gitignored, so on a fresh clone there is nothing to
	// serve. The dispatch assertion below still holds and still matters; only the
	// 200 depends on a built bundle.
	_, statErr := os.Stat(filepath.Join("frontend-react", "dist"))
	h := newHarness(t)

	for _, path := range []string{"/assets/driftwood.css", "/thresholds.js"} {
		before := h.hitCount()
		resp := h.do(t, http.MethodGet, proxy.ControlPrefix+path, nil)

		if got := h.hitCount(); got != before {
			t.Errorf("GET %s: asset was proxied to the backend", path)
		}
		if statErr != nil {
			if resp.StatusCode == http.StatusOK {
				t.Errorf("GET %s: no dist/, yet status = 200", path)
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, resp.StatusCode)
		}
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			return sb.String()
		}
	}
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
