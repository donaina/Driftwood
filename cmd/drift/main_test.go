package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	// NewStore resolves its persistence paths from $HOME, so point it at a temp
	// directory rather than the developer's real ~/.driftwood.
	t.Setenv("HOME", t.TempDir())
	s, err := storage.NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// The seed is the only contract Driftwood invents, and it may only invent one
// for an endpoint it serves itself. A fabricated baseline for a path on the
// user's own API made the first healthy request to that path report
// BREAKING / REMOVED_FIELD for a field that never existed.
func TestSeedDemoBaselines_OnlySeedsItsOwnMock(t *testing.T) {
	store := newTestStore(t)
	seedDemoBaselines(store)

	all := store.GetAllBaselines(store.ActiveProject())
	if len(all) != 1 {
		t.Fatalf("seeded %d baselines, want exactly 1", len(all))
	}
	seeded, ok := store.GetBaseline(store.ActiveProject(), "GET", demoBaselinePath)
	if !ok {
		t.Fatalf("the mock endpoint %s was not seeded", demoBaselinePath)
	}
	if !strings.HasPrefix(seeded.Path, proxy.ControlPrefix+"/") {
		t.Errorf("seeded %s %s, which is outside the control namespace — Driftwood does not serve it",
			seeded.Method, seeded.Path)
	}
	if _, exists := store.GetBaseline(store.ActiveProject(), "GET", "/api/users"); exists {
		t.Error("a baseline was seeded for /api/users, a path on the user's own API")
	}
}

func TestSeedDemoBaselines_DoesNotOverwriteAnExistingContract(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.SaveBaseline(store.ActiveProject(), "GET", demoBaselinePath, `{"id": 1, "mine": true}`); err != nil {
		t.Fatalf("seeding a baseline to protect: %v", err)
	}

	seedDemoBaselines(store)

	got, _ := store.GetBaseline(store.ActiveProject(), "GET", demoBaselinePath)
	if !strings.Contains(got.SamplePayload, "mine") {
		t.Errorf("the existing contract was overwritten by the demo seed: %s", got.SamplePayload)
	}
	if got.Version != 1 {
		t.Errorf("version = %d after seeding, want 1 (seeding must not add a version)", got.Version)
	}
}

// The four ResolveImportProject cases moved to internal/storage with the
// function itself, when the import route needed the same answer the CLI gives.

// A hint file naming an address nothing answers on must not be mistaken for a
// running instance.
//
// This is the state a killed instance leaves — the shutdown goroutine that
// clears the hint never ran — and it is the state a machine reaches most often,
// since Ctrl-C on a wedged process is not a graceful shutdown. Reading the hint
// as fact would send every import to a dead address and fail, so the caller has
// to confirm it.
func TestLiveInstanceIgnoresAHintNothingAnswers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// A port with nothing on it: bound to learn a free one, then released. The
	// alternative is a hard-coded port that is somebody else's on another machine.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	dead := "http://" + ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("releasing the port: %v", err)
	}

	if err := storage.WriteInstance(storage.InstanceInfo{ControlURL: dead, PID: os.Getpid()}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	if url, ok := liveInstance(); ok {
		t.Errorf("liveInstance() = %q, want no instance: the hint names %s and nothing is listening there", url, dead)
	}
}

// And a hint is only accepted from something that identifies itself as
// Driftwood.
//
// The port is reusable and the hint outlives the process, so the address it
// names can easily be answering as something else by the time an import reads
// it — a dev server, or an unrelated instance of a different program. A bare
// 200 is not enough: handing a spec to a stranger, or reading the import route
// of an older build as if it were this one's, is the failure this check exists
// to prevent.
func TestLiveInstanceRejectsSomethingElseOnThePort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stranger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"app":"something-else"}`)
	}))
	defer stranger.Close()

	if err := storage.WriteInstance(storage.InstanceInfo{ControlURL: stranger.URL, PID: os.Getpid()}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	if url, ok := liveInstance(); ok {
		t.Errorf("liveInstance() = %q, want no instance: %s answers, but not as Driftwood", url, stranger.URL)
	}
}

func TestLiveInstanceFindsARunningInstance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var asked string
	real := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"app":"driftwood","active_project":"default"}`)
	}))
	defer real.Close()

	if err := storage.WriteInstance(storage.InstanceInfo{ControlURL: real.URL, PID: os.Getpid()}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	url, ok := liveInstance()
	if !ok {
		t.Fatalf("liveInstance() found nothing at %s", real.URL)
	}
	if url != real.URL {
		t.Errorf("liveInstance() = %q, want %q", url, real.URL)
	}
	// The probe has to be the control route, not "/": a target answering 200 at
	// its root would otherwise pass for an instance.
	if asked != proxy.ControlPrefix+"/api/instance" {
		t.Errorf("probed %q, want %q", asked, proxy.ControlPrefix+"/api/instance")
	}
}

// No hint at all is the ordinary case on a machine where nothing has run, and
// it must not be reported as a running instance.
func TestLiveInstanceWithNoHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if url, ok := liveInstance(); ok {
		t.Errorf("liveInstance() = %q with no hint written, want no instance", url)
	}
}

// A guard on the harness rather than on the product: if the store ever resolves
// its persistence directory from something other than $HOME — os/user.Current(),
// say, which ignores the variable — every test in this file would start writing
// into the developer's real ~/.driftwood with no visible symptom. This fails
// loudly instead.
func TestSeedingDoesNotTouchTheRealHome(t *testing.T) {
	realHome, err := os.UserHomeDir() // before t.Setenv, so this is the real one
	if err != nil {
		t.Skip("no home directory to protect")
	}
	realState := filepath.Join(realHome, ".driftwood", "baselines.json")
	before, beforeErr := os.Stat(realState)

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := storage.NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	seedDemoBaselines(store)

	// The store must have read the redirected HOME, or the redirect is fiction
	// and this test proves nothing.
	if _, err := os.Stat(filepath.Join(home, ".driftwood", "baselines.json")); err != nil {
		t.Fatalf("nothing persisted under the redirected home: %v", err)
	}

	after, afterErr := os.Stat(realState)
	if beforeErr == nil && afterErr == nil && !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("%s was modified during the test", realState)
	}
	if beforeErr != nil && afterErr == nil {
		t.Errorf("the test created %s in the real home directory", realState)
	}
}

// An active project with no target must start, not end the process.
//
// This was a production crash loop: a project with no backend, made active,
// killed Driftwood on every restart with "invalid target URL: target URL cannot
// be empty" — taking every other project's monitoring down with the one that
// had no backend. The test lives here rather than beside the proxy because the
// fatal call was in main, and a log.Fatalf is exactly the kind of branch a test
// cannot get past: which is why this one is written against the decision main
// makes, extracted into buildProxy so it can be reached.
//
// A targetless project is not a malformed store. CreateProject accepts an empty
// target_url and says so, and the request path already answers such a project
// with a 502, so startup refusing it was the outlier rather than the rule.
func TestBuildProxyServesAnActiveProjectWithNoTarget(t *testing.T) {
	store := newTestStore(t)
	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	project, err := store.CreateProject("Acme")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := store.SetActiveProject(project.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	// What main actually reads. GetConfig overlays the active project's target,
	// so this is the empty string that used to reach parseAndValidateTarget — and
	// it is empty regardless of --target, because the overlay replaces the flag's
	// value rather than falling back to it.
	target := store.GetConfig().TargetURL
	if target != "" {
		t.Fatalf("the active project has target %q, so this is not the empty case", target)
	}

	prx, err := buildProxy(target, store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("buildProxy refused a targetless active project, which is the crash this guards: %v", err)
	}

	// And it serves rather than merely constructing: the same honest 502 the
	// request path gives any other project with nowhere to dial.
	srv := httptest.NewServer(prx.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/users")
	if err != nil {
		t.Fatalf("request through the targetless proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no target configured") {
		t.Errorf("body = %q, want it to name the missing target", body)
	}

	// --target seeds the first project; it is not a standing override that
	// refills a project the operator left empty. Promoting the flag's value here
	// would silently point a client at another client's backend.
	if got, _, _ := store.ProjectTarget(project.ID); got != "" {
		t.Errorf("the targetless project was given %q, so the flag became a standing override", got)
	}
}
