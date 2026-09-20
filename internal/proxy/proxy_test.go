package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

// isolateHome points the store at a throwaway home directory.
//
// storage.NewStore resolves its persist path from the home directory, and the
// proxy auto-saves a baseline for the first JSON response it sees — so without
// this the suite reads the developer's real ~/.driftwood and writes test
// endpoints into it. GET:/api/test was sitting in a real baselines.json, put
// there by TestSanitizeTrafficWired. internal/server and cmd/drift have isolated
// HOME for exactly this reason; these tests had not.
//
// A temp HOME is also what makes the assertions exact: a developer who already
// has a baseline for /api/test gets different behaviour from one who does not.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestProxySSRFValidation(t *testing.T) {
	isolateHome(t)
	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Test: valid http URL
	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("valid http URL should be accepted: %v", err)
	}
	if prx.SetTarget("https://api.example.com", false) != nil {
		t.Errorf("valid https URL should be accepted")
	}

	// Test: invalid schemes should be rejected
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://localhost:8787",   // SSRF to local metadata - should be blocked
		"http://169.254.169.254/", // AWS metadata
		"http://127.0.0.1:8080",   // local
		"",                        // empty
		"not-a-url",
		// The same three destinations in spellings that defeated the old
		// string comparison. Each reached loopback or a metadata service when
		// the check was a plain equality test against a lowercase dotted quad.
		"http://LOCALHOST:8787",
		"http://localhost.:8787", // trailing dot: fully qualified, same host
		"http://0.0.0.0:8787",    // unspecified; dials loopback on Linux
		"http://2130706433/",     // decimal 127.0.0.1
		"http://0x7f000001/",     // hex 127.0.0.1
		"http://fd00::1/",        // IPv6 ULA
		"http://metadata.google.internal/",
		"http://intranet", // dotless: resolved against the search domains
	}

	for _, u := range invalidURLs {
		err = prx.SetTarget(u, false)
		if err == nil {
			t.Errorf("SSRF URL should be rejected: %s", u)
		}
	}

	// Test: valid URL after invalid
	if err := prx.SetTarget("https://api.github.com", false); err != nil {
		t.Errorf("valid URL after invalid should work: %v", err)
	}
}

func TestProxyTargetURLRaceSafety(t *testing.T) {
	isolateHome(t)
	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent SetTarget and Handler access should not panic
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			prx.SetTarget("https://api.example.com", false)
			_ = prx.Handler()
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestSanitizeTrafficWired(t *testing.T) {
	isolateHome(t)
	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Use test proxy for localhost target
	prx, err := NewProxyAllowPrivate("http://localhost:3000", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test server that returns JSON with auth headers
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret123; HttpOnly")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 1, "name": "test"}`))
	}))
	defer targetServer.Close()

	// Point proxy to test server
	targetURL, _ := url.Parse(targetServer.URL)
	prx.GetTargetURLForTest().Store(targetURL)

	// Make request through proxy
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret456")

	rec := httptest.NewRecorder()
	prx.Handler()(rec, req)

	// Check that traffic was captured and sanitized
	// The stored traffic should have redacted headers
	t.Logf("Response status: %d", rec.Code)
}

// TestBreakingAlertCarriesItsExplanation covers a path that had no test at all,
// which is why it was broken in three places at once and nobody noticed.
//
// The contract: the alert is published the moment the break is detected and
// does not wait for the sidecar; the explanation is filed against the stored
// alert; and it is announced separately, once it exists. None of that held
// before. The write used to land in a map Publish had already marshalled, so
// the explanation reached neither the wire nor the store, and the Alerts view
// read three fields no alert has ever had.
func TestBreakingAlertCarriesItsExplanation(t *testing.T) {
	isolateHome(t)

	/* A stub for the sidecar. The body is the shape ai/src/types.ts declares,
	   and it travels on a channel rather than a shared variable so the handler
	   goroutine and this one are ordered by something the race detector can
	   see. */
	sidecarBodies := make(chan map[string]interface{}, 4)
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sidecarBodies <- body

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":"count became a string",` +
			`"impact":"parsers expecting a number fail",` +
			`"root_cause":"a column type change",` +
			`"suggested_action":"revert the migration",` +
			`"confidence":0.9}`))
	}))
	defer sidecar.Close()

	// Written before any request, so the goroutine that reads it is ordered
	// after this line by the go statement that spawns it.
	oldExplainURL := explainURL
	explainURL = sidecar.URL
	t.Cleanup(func() { explainURL = oldExplainURL })

	// The first response is captured as the baseline; the second breaks it.
	var hits int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt64(&hits, 1) == 1 {
			_, _ = w.Write([]byte(`{"count":1,"token":"sk-live-must-not-leave"}`))
			return
		}
		_, _ = w.Write([]byte(`{"count":"one","token":"sk-live-must-not-leave"}`))
	}))
	defer backend.Close()

	store, err := storage.NewStore(backend.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	prx, err := NewProxyAllowPrivate(backend.URL, store, hub, &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe over the same SSE endpoint the dashboard uses, so this covers
	// delivery rather than only the hub's memory.
	type broadcast struct {
		Type string                 `json:"type"`
		Data map[string]interface{} `json:"data"`
	}
	received := make(chan broadcast, 32)

	sse := httptest.NewServer(http.HandlerFunc(hub.SSEHandler))
	defer sse.Close()

	stream, err := http.Get(sse.URL)
	if err != nil {
		t.Fatalf("could not open the event stream: %v", err)
	}
	defer stream.Body.Close()

	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue // the "event: ping" line, and the blank separators
			}
			var b broadcast
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &b); err == nil && b.Type != "" {
				received <- b
			}
		}
	}()

	// waitFor returns the next event of this type, discarding the others.
	waitFor := func(eventType string) broadcast {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case b := <-received:
				if b.Type == eventType {
					return b
				}
			case <-deadline:
				t.Fatalf("timed out waiting for a %q event", eventType)
				return broadcast{}
			}
		}
	}

	request := func() {
		req := httptest.NewRequest(http.MethodGet, "/thing", nil)
		prx.Handler()(httptest.NewRecorder(), req)
	}

	request() // establishes the baseline
	request() // breaks it

	alert := waitFor("alert")
	if got := alert.Data["contract_status"]; got != "BREAKING" {
		t.Fatalf("expected a BREAKING alert, got %v", got)
	}

	/* The alert must not be carrying an explanation, and not because the
	   sidecar is slow — nothing writes that key into the published map any
	   more. The alert is the notification, so it goes out immediately; waiting
	   on an optional paragraph would put a network round trip in front of every
	   breaking change. */
	if _, present := alert.Data["ai_explanation"]; present {
		t.Error("the published alert carried an explanation; it must not wait for the sidecar")
	}

	explained := waitFor("alert_explained")
	if explained.Data["traffic_id"] != alert.Data["traffic_id"] {
		t.Errorf("alert_explained names %v, but the alert was %v",
			explained.Data["traffic_id"], alert.Data["traffic_id"])
	}

	alerts := store.GetAlerts(store.ActiveProject(), 10)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 stored alert, got %d", len(alerts))
	}
	if alerts[0].AIExplanation == nil {
		t.Fatal("the stored alert carries no explanation: the sidecar answered but nothing filed it")
	}
	if got := alerts[0].AIExplanation["summary"]; got != "count became a string" {
		t.Errorf("stored explanation summary = %v", got)
	}

	/* The alert is one endpoint breaking, with the per-field detail under Diff.
	   These are the paths the Alerts view reads, asserted where they are
	   produced rather than only in a browser. */
	if alerts[0].Diff == nil || len(alerts[0].Diff.Deltas) == 0 {
		t.Fatal("the alert carries no deltas, so the Alerts view has nothing to render")
	}
	for _, d := range alerts[0].Diff.Deltas {
		if d.JSONPath == "" || d.Severity == "" || d.Message == "" {
			t.Errorf("delta is missing a field the Alerts view reads: %+v", d)
		}
	}
	if alerts[0].Endpoint != "GET /thing" {
		t.Errorf("alert endpoint = %q", alerts[0].Endpoint)
	}

	/* Both samples leave the process for a third-party API. The baseline is
	   stored raw, so the copy that must be clean is the one sent. */
	var sidecarBody map[string]interface{}
	select {
	case sidecarBody = <-sidecarBodies:
	case <-time.After(time.Second):
		t.Fatal("the sidecar was never called")
	}
	for _, field := range []string{"baseline_sample", "current_sample"} {
		sample, ok := sidecarBody[field].(string)
		if !ok {
			t.Errorf("the sidecar was sent no %s: %v", field, sidecarBody)
			continue
		}
		if strings.Contains(sample, "sk-live-must-not-leave") {
			t.Errorf("the %s reached the sidecar unredacted: %s", field, sample)
		}
	}
	if sidecarBody["endpoint"] != "GET /thing" {
		t.Errorf("the sidecar was told about %v", sidecarBody["endpoint"])
	}
}

/*
A breaking change must not make the client wait on the sidecar.

	explainAsync is named for what it is meant to do, is documented as returning
	immediately, and the block that calls it reasons at length about what "the
	sidecar takes up to 8s" would cost the caller. None of that held while the
	call had no `go`: the handler could not return until the sidecar answered.

	Measured through a real listener rather than a ResponseRecorder, because the
	question is what a client sees and a recorder cannot show it. Reading the body
	to EOF is deliberate — it is what cannot finish until the handler has, if the
	response is still sitting in the server's write buffer.
*/
func TestBreakingChangeDoesNotWaitOnTheSidecar(t *testing.T) {
	isolateHome(t)

	// Buffered, so the handler never blocks announcing itself.
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }

	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":"held open"}`))
	}))
	/* LIFO, and the order is load-bearing: Close waits for the handler still
	   sitting on <-release, so unblocking has to be registered after it and
	   therefore run first. The other order hangs the suite on the failing path —
	   the one moment the failure message is worth having. */
	t.Cleanup(sidecar.Close)
	t.Cleanup(unblock)

	oldExplainURL := explainURL
	explainURL = sidecar.URL
	t.Cleanup(func() { explainURL = oldExplainURL })

	// The first response becomes the baseline; the second breaks it.
	var hits int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt64(&hits, 1) == 1 {
			_, _ = w.Write([]byte(`{"count":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"count":"one"}`))
	}))
	defer backend.Close()

	store, err := storage.NewStore(backend.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	prx, err := NewProxyAllowPrivate(backend.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	proxySrv := httptest.NewServer(prx.Handler())
	defer proxySrv.Close()

	// No t.Fatal below the goroutine boundary: it belongs to the test goroutine.
	fetch := func() error {
		resp, err := http.Get(proxySrv.URL + "/thing")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}

	if err := fetch(); err != nil {
		t.Fatalf("establishing the baseline: %v", err)
	}

	type result struct {
		took time.Duration
		err  error
	}
	done := make(chan result, 1)
	go func() {
		start := time.Now()
		err := fetch()
		done <- result{time.Since(start), err}
	}()

	// Without this the test proves nothing: a proxy that never consulted the
	// sidecar would satisfy the assertion below for entirely the wrong reason.
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the sidecar was never called, so there was nothing to wait on")
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("the breaking request failed: %v", r.err)
		}
		if r.took > 2*time.Second {
			t.Errorf("the breaking request took %s with the sidecar held open: the client "+
				"waited on a service this product is built to work without", r.took)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the breaking request had not returned 3s in, with the sidecar held open: " +
			"explainAsync is being called synchronously")
	}

	/* Let the sidecar answer, then wait for the explanation to be filed.

	   Two reasons, and the second is not obvious. It asserts the `go` made the
	   call concurrent rather than absent — a dropped explanation passes every
	   timing assertion above. And it puts the read of explainURL inside the
	   goroutine before a lock the test also takes, which gives the race detector
	   the edge it needs; the write in the cleanup above is otherwise unsynchronized
	   against it. */
	unblock()

	deadline := time.Now().Add(5 * time.Second)
	for {
		alerts := store.GetAlerts(store.ActiveProject(), 10)
		if len(alerts) == 1 && alerts[0].AIExplanation != nil {
			if got := alerts[0].AIExplanation["summary"]; got != "held open" {
				t.Errorf("stored explanation summary = %v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the sidecar answered but nothing filed the explanation: %d alerts stored", len(alerts))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

/*
A backend that fails is not a backend whose contract changed.

	Diffing an error body against a baseline captured from a success reports
	every field of the real response as REMOVED_FIELD and alerts on all of them,
	so one transient 500 arrives as a page of confident nonsense naming fields
	that never went anywhere. The status is the finding; the body is not.
*/
func TestFailedResponseIsNotDiffedAgainstTheContract(t *testing.T) {
	isolateHome(t)

	var mu sync.Mutex
	seen := map[string]int{}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		mu.Lock()
		seen[r.URL.Path]++
		attempt := seen[r.URL.Path]
		mu.Unlock()

		// Fails from the first call, so it can never become a contract.
		if r.URL.Path == "/fails-first" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}

		// Every other endpoint answers healthily once, which is what makes the
		// baseline a success contract, and then does as its name says.
		if attempt == 1 {
			_, _ = w.Write([]byte(`{"id":1,"nick":"al"}`))
			return
		}
		switch r.URL.Path {
		case "/breaks-to-500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom","trace":"x"}`))
		case "/breaks-to-404":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		default:
			_, _ = w.Write([]byte(`{"id":1,"nick":"al"}`))
		}
	}))
	defer backend.Close()

	store, err := storage.NewStore(backend.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	prx, err := NewProxyAllowPrivate(backend.URL, store, hub, &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	request := func(path string) {
		t.Helper()
		prx.Handler()(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	// Matching on the status rather than on position keeps the lookup honest
	// about which of the two sightings it wants.
	failure := func(path string, code int) types.CapturedTraffic {
		t.Helper()
		for _, tr := range store.GetTraffics(store.ActiveProject(), 100) {
			if tr.Path == path && tr.StatusCode == code {
				return tr
			}
		}
		t.Fatalf("no %d response was recorded for %s", code, path)
		return types.CapturedTraffic{}
	}

	request("/breaks-to-500") // healthy, so this becomes the contract
	request("/breaks-to-500") // then the endpoint falls over

	failed := failure("/breaks-to-500", http.StatusInternalServerError)
	if failed.ContractStatus != "BREAKING" {
		t.Errorf("a 500 against a healthy contract reads as %q, want BREAKING", failed.ContractStatus)
	}
	if failed.Diff == nil || len(failed.Diff.Deltas) == 0 {
		t.Fatal("the failure produced no deltas at all")
	}
	for _, d := range failed.Diff.Deltas {
		if d.Kind == types.KindRemovedField || d.Kind == types.KindAddedField ||
			d.Kind == types.KindTypeMismatch {
			t.Errorf("the error body was diffed against the contract: %s %s", d.Kind, d.JSONPath)
		}
	}
	if got := failed.Diff.Deltas[0].Kind; got != types.KindStatusCodeChange {
		t.Errorf("the delta describing a failure is %q, want %q", got, types.KindStatusCodeChange)
	}

	/* One breaking alert, and it is the right one: a service that has started
	   failing is worth waking someone for. What must not happen is the
	   field-by-field version that used to accompany it. */
	breaking := 0
	for _, a := range store.GetAlerts(store.ActiveProject(), 50) {
		if a.ContractStatus != "BREAKING" {
			continue
		}
		breaking++
		if a.Diff == nil {
			t.Fatal("the breaking alert carries no diff")
		}
		for _, d := range a.Diff.Deltas {
			if d.Kind == types.KindRemovedField || d.Kind == types.KindTypeMismatch {
				t.Errorf("the alert reports %s at %s because the backend errored", d.Kind, d.JSONPath)
			}
		}
	}
	if breaking != 1 {
		t.Fatalf("expected 1 breaking alert for one failing endpoint, got %d", breaking)
	}

	/* A 4xx is the contract being enforced on the caller rather than the API
	   changing shape. It is filed as a warning — visible in the alerts view,
	   because an endpoint that stopped answering 200 is worth seeing — but it
	   is never reported as the contract breaking. */
	request("/breaks-to-404")
	request("/breaks-to-404")

	rejected := failure("/breaks-to-404", http.StatusNotFound)
	if rejected.ContractStatus != "WARNING" {
		t.Errorf("a 404 against a healthy contract reads as %q, want WARNING", rejected.ContractStatus)
	}
	filed := false
	for _, a := range store.GetAlerts(store.ActiveProject(), 50) {
		if !strings.HasSuffix(a.Endpoint, "/breaks-to-404") {
			continue
		}
		filed = true
		if a.ContractStatus != "WARNING" {
			t.Errorf("the 404 was filed as %q, want WARNING", a.ContractStatus)
		}
		if a.Diff == nil {
			t.Fatal("the 404 alert carries no diff")
		}
		for _, d := range a.Diff.Deltas {
			if d.Kind != types.KindStatusCodeChange {
				t.Errorf("the 404 was diffed field by field: %s %s", d.Kind, d.JSONPath)
			}
		}
	}
	if !filed {
		t.Error("the 404 was recorded but filed nowhere, so nothing shows it happened")
	}

	/* And an error body must never be adopted as the contract, or the failure
	   becomes the baseline and every later success reads as drift. */
	request("/fails-first")
	if _, exists := store.GetBaseline(store.ActiveProject(), "GET", "/fails-first"); exists {
		t.Error("an error response was saved as the contract")
	}
}

// TestResponseLargerThanTheAnalysisCapArrivesWhole covers the cap as a bound on
// analysis rather than on traffic.
//
// The capped copy of the body used to be the copy that was served, so a response
// over the limit reached the client truncated while its Content-Length still
// described the original. A proxy that silently corrupts the traffic it is
// watching is worse than one that does not watch it. The client here is a real
// one over a real socket, because that is what turns the mismatch into an error
// someone would see rather than a length nobody checks.
func TestResponseLargerThanTheAnalysisCapArrivesWhole(t *testing.T) {
	isolateHome(t)

	const bodySize = maxAnalyzedBody + 4096
	body := strings.Repeat("x", bodySize)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(bodySize))
		_, _ = io.WriteString(w, body)
	}))
	defer backend.Close()

	store, err := storage.NewStore(backend.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	prx, err := NewProxyAllowPrivate(backend.URL, store,
		events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	front := httptest.NewServer(prx.Handler())
	defer front.Close()

	resp, err := http.Get(front.URL + "/big")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the proxied response: %v", err)
	}
	if len(got) != bodySize {
		t.Errorf("the client received %d bytes of a %d-byte response; the analysis cap truncated the traffic",
			len(got), bodySize)
	}
}

// zeroReader yields an endless run of zero bytes, which compresses extremely
// well — the point of the fixture below.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// TestGzippedResponseIsForwardedCompressedAndAnalyzedBounded covers both halves
// of the gzip handling, which are not the same half.
//
// A gzipped body is decompressed in order to be read, not in order to be served:
// rewriting it to identity while leaving Content-Encoding: gzip in place handed
// a client that had asked for gzip a body it would then fail to decode. And the
// expansion is where a large or hostile response turns a small read into a large
// allocation, so it is capped on the expanded side — the compressed side is
// already capped, and a ratio of 1000:1 makes that cap bound nothing.
func TestGzippedResponseIsForwardedCompressedAndAnalyzedBounded(t *testing.T) {
	isolateHome(t)

	// 64 MiB of zeros compresses to a few tens of KB, far under the cap on the
	// compressed side, so nothing here is bounded unless the expansion is.
	const expanded = 64 << 20
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := io.CopyN(zw, zeroReader{}, expanded); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	gz := compressed.Bytes()
	if len(gz) >= maxAnalyzedBody {
		t.Fatalf("the fixture is not a compression bomb: %d compressed bytes", len(gz))
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(gz)))
		_, _ = w.Write(gz)
	}))
	defer backend.Close()

	store, err := storage.NewStore(backend.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	prx, err := NewProxyAllowPrivate(backend.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	// Driven through the handler rather than a socket: the assertion below is on
	// what the proxy recorded, and a real client can finish reading its body
	// before the handler has finished filing the transaction.
	req := httptest.NewRequest(http.MethodGet, "/bomb", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	prx.Handler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q: the client asked for gzip and its body was rewritten underneath it", got)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, gz) {
		t.Errorf("the client received %d bytes, want the %d gzipped bytes the backend sent", len(got), len(gz))
	}

	traffics := store.GetTraffics(store.ActiveProject(), 1)
	if len(traffics) != 1 {
		t.Fatalf("recorded %d transactions, want 1", len(traffics))
	}
	if n := len(traffics[0].ResponseBody); n > maxAnalyzedBody {
		t.Errorf("the analysis retained %d bytes of a %d-byte expansion; the decompression is unbounded", n, expanded)
	}
}
