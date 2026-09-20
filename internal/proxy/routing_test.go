package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
)

// namingBackend answers every request with its own name, in plain text.
//
// Plain text rather than JSON on purpose: a JSON response would be auto-saved as
// a baseline, and these tests are about where a request is delivered, not about
// what is recorded. Leaving the store's endpoint maps untouched keeps a routing
// failure from being able to look like a baseline failure.
func namingBackend(t *testing.T, name string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, name)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialThrough sends one request through the proxy's handler and returns the
// body the backend answered with.
func dialThrough(t *testing.T, prx *Proxy, path string) string {
	t.Helper()
	front := httptest.NewServer(prx.Handler())
	defer front.Close()

	resp, err := http.Get(front.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// TestSwitchingProjectRetargetsTheProxy is the test the per-project target
// exists for: switching project changes which backend requests reach.
//
// Nothing here calls RefreshRouting or SetProjectTarget after the switch. The
// proxy is not told — the store's active project changes, and the next request
// has to find its way to the other backend by itself. A snapshot that has to be
// refreshed by its mutators passes every test that refreshes it and fails this
// one, which is the point: the way this breaks in production is a switch that
// nobody wired up, delivering the next request to the previous client's backend.
func TestSwitchingProjectRetargetsTheProxy(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	alpha := namingBackend(t, "alpha")
	beta := namingBackend(t, "beta")

	// The constructor writes its target into the store's first project, so this
	// is also the assertion that a fresh install proxies to --target.
	prx, err := NewProxyAllowPrivate(alpha.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}
	first := store.ActiveProject()

	second, err := store.CreateProject("Client B")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := prx.SetProjectTarget(second.ID, beta.URL, true); err != nil {
		t.Fatalf("SetProjectTarget(%s): %v", second.ID, err)
	}

	if got := dialThrough(t, prx, "/anything"); got != "alpha" {
		t.Fatalf("before the switch the proxy answered %q, want %q", got, "alpha")
	}
	if first == second.ID {
		t.Fatalf("CreateProject returned the id already active (%q); nothing was switched", first)
	}

	// The switch. The proxy is deliberately not informed.
	if err := store.SetActiveProject(second.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	if got := dialThrough(t, prx, "/anything"); got != "beta" {
		t.Errorf("after switching to project %s the proxy answered %q, want %q — requests are being "+
			"delivered to the previous project's backend", second.ID, got, "beta")
	}

	// And back, so this is a property of the switch rather than of which
	// project happens to be second.
	if err := store.SetActiveProject(first); err != nil {
		t.Fatalf("SetActiveProject(%s): %v", first, err)
	}
	if got := dialThrough(t, prx, "/anything"); got != "alpha" {
		t.Errorf("after switching back the proxy answered %q, want %q", got, "alpha")
	}
}

// TestARetargetedProjectIsPickedUpWithoutAnExplicitRefresh covers the other
// routing change a request can arrive after: the active project stays put and
// its target moves.
func TestARetargetedProjectIsPickedUpWithoutAnExplicitRefresh(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	alpha := namingBackend(t, "alpha")
	beta := namingBackend(t, "beta")

	prx, err := NewProxyAllowPrivate(alpha.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}
	if got := dialThrough(t, prx, "/anything"); got != "alpha" {
		t.Fatalf("initial target answered %q, want %q", got, "alpha")
	}

	// The store is retargeted directly, so the proxy is not told.
	if err := store.SetProjectTarget(store.ActiveProject(), beta.URL, true); err != nil {
		t.Fatalf("SetProjectTarget: %v", err)
	}

	if got := dialThrough(t, prx, "/anything"); got != "beta" {
		t.Errorf("after retargeting the active project the proxy answered %q, want %q", got, "beta")
	}
}

// TestEachProjectsTargetIsIndependent checks that targets are held per project
// rather than in one shared slot, and that the SSRF decision travels with the
// target it was made for.
func TestEachProjectsTargetIsIndependent(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	alpha := namingBackend(t, "alpha")
	beta := namingBackend(t, "beta")

	prx, err := NewProxyAllowPrivate(alpha.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}
	first := store.ActiveProject()

	second, err := store.CreateProject("Client B")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := prx.SetProjectTarget(second.ID, beta.URL, true); err != nil {
		t.Fatalf("SetProjectTarget: %v", err)
	}

	// Retargeting B left A alone, and each project reports its own target back.
	gotFirst, _, _ := store.ProjectTarget(first)
	gotSecond, allowPrivateSecond, _ := store.ProjectTarget(second.ID)
	if gotFirst != alpha.URL {
		t.Errorf("project %s targets %q, want %q — one project's target overwrote another's", first, gotFirst, alpha.URL)
	}
	if gotSecond != beta.URL {
		t.Errorf("project %s targets %q, want %q", second.ID, gotSecond, beta.URL)
	}
	if !allowPrivateSecond {
		t.Errorf("project %s was retargeted with allowPrivate=true but reports false; the SSRF "+
			"decision has to persist with the target it was made for", second.ID)
	}
}

// TestARefusedTargetLeavesRoutingUntouched is the property that makes the
// validation order matter: the target is checked before the store is written, so
// a refused retarget is not a retarget that half-happened.
func TestARefusedTargetLeavesRoutingUntouched(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	alpha := namingBackend(t, "alpha")

	prx, err := NewProxyAllowPrivate(alpha.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}
	active := store.ActiveProject()

	// allowPrivate=false is what a request from off-box gets. The httptest
	// backend is on loopback, so this is exactly the retarget that must not be
	// allowed to point the proxy at it.
	if err := prx.SetProjectTarget(active, alpha.URL, false); err == nil {
		t.Fatal("a loopback target was accepted for a non-loopback caller")
	} else if !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("refusal error is %v, want it to wrap ErrInvalidTarget so the API can "+
			"answer 400 rather than 500", err)
	}

	if got := dialThrough(t, prx, "/anything"); got != "alpha" {
		t.Errorf("after a refused retarget the proxy answered %q, want %q — a rejected target "+
			"changed where requests go", got, "alpha")
	}
	if stored, _, _ := store.ProjectTarget(active); stored != alpha.URL {
		t.Errorf("after a refused retarget the store holds %q, want %q", stored, alpha.URL)
	}
}

// TestAnUnknownProjectCannotBeRetargeted closes the path from the API's project
// parameter to the store's project map. Without the existence check the store
// would conjure the project into being by writing a target for it.
func TestAnUnknownProjectCannotBeRetargeted(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	prx, err := NewProxyAllowPrivate("http://127.0.0.1:1", store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}

	err = prx.SetProjectTarget("never-created", "https://example.com", false)
	if err == nil {
		t.Fatal("retargeting a project that does not exist was accepted")
	}
	if store.ProjectExists("never-created") {
		t.Error("retargeting a project that does not exist created it")
	}
}

// TestAProjectWithNoTargetHasNowhereToGo covers a state that did not exist
// before projects: a project that exists and has never been given a target.
//
// It has to answer honestly rather than fall back to another project's backend.
// Falling back would be the misattribution this whole design is arranged to
// prevent — a request arriving under one client's name and being served by
// another's backend, with nothing in the response saying so.
func TestAProjectWithNoTargetHasNowhereToGo(t *testing.T) {
	isolateHome(t)

	store, err := storage.NewStore("http://localhost:8787", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	alpha := namingBackend(t, "alpha")

	prx, err := NewProxyAllowPrivate(alpha.URL, store, events.NewHub(), &mock.MockController{})
	if err != nil {
		t.Fatalf("NewProxyAllowPrivate: %v", err)
	}

	blank, err := store.CreateProject("Not Configured Yet")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if target, _, _ := store.ProjectTarget(blank.ID); target != "" {
		t.Fatalf("a new project already has target %q; this test assumes it starts unset", target)
	}
	if err := store.SetActiveProject(blank.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	front := httptest.NewServer(prx.Handler())
	defer front.Close()

	resp, err := http.Get(front.URL + "/anything")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("an untargeted project answered %d, want 502", resp.StatusCode)
	}
	if string(body) == "alpha" {
		t.Error("an untargeted project fell back to another project's backend")
	}
}
