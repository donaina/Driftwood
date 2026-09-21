package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

// storageTestTraffic is one recorded request, built directly so a test about
// which project a read serves does not depend on how traffic is raised.
func storageTestTraffic(projectID, path string) types.CapturedTraffic {
	return types.CapturedTraffic{
		ID:             projectID + path,
		ProjectID:      projectID,
		Method:         http.MethodGet,
		Path:           path,
		ContractStatus: "NO_BASELINE",
	}
}

// decodeBody reads a JSON response body into v.
func decodeBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
}

// listAfter runs a request and returns the project list the server answered
// with, which every project route returns in the same shape.
func (h *harness) listAfter(t *testing.T, resp *http.Response) projectList {
	t.Helper()
	var got projectList
	decodeBody(t, resp, &got)
	return got
}

// TestProjectListReportsTheActiveProject is the route the switcher reads on load.
func TestProjectListReportsTheActiveProject(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, "/_driftwood/api/projects", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := h.listAfter(t, resp)

	if got.Active != h.store.ActiveProject() {
		t.Errorf("active = %q, want %q", got.Active, h.store.ActiveProject())
	}
	if len(got.Projects) != 1 {
		t.Fatalf("got %d projects, want the one a fresh install has", len(got.Projects))
	}
	if got.Projects[0].ID != got.Active {
		t.Errorf("the only project is %q but the active one is %q; a fresh install should be looking at its own",
			got.Projects[0].ID, got.Active)
	}
}

// TestCreateProjectRouteCarriesItsTarget covers the create path end to end: the
// project exists afterwards, and so does its target.
func TestCreateProjectRouteCarriesItsTarget(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, "/_driftwood/api/projects/create",
		`{"name":"Client B","target_url":"http://localhost:4242"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var created struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		TargetURL string `json:"target_url"`
	}
	decodeBody(t, resp, &created)

	if created.Name != "Client B" {
		t.Errorf("name = %q, want %q", created.Name, "Client B")
	}
	if created.TargetURL != "http://localhost:4242" {
		t.Errorf("target = %q, want the posted value; the route accepted a target and did not record it",
			created.TargetURL)
	}
	if !h.store.ProjectExists(created.ID) {
		t.Errorf("project %q was returned but does not exist", created.ID)
	}

	list := h.listAfter(t, h.do(t, http.MethodGet, "/_driftwood/api/projects", nil))
	if len(list.Projects) != 2 {
		t.Errorf("got %d projects after creating one, want 2", len(list.Projects))
	}
}

// TestCreateProjectRefusesAnInvalidTarget asserts the ordering the route is
// built around: the target is checked before the project is made, so a refused
// target leaves no half-made project behind.
func TestCreateProjectRefusesAnInvalidTarget(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, "/_driftwood/api/projects/create",
		`{"name":"Client B","target_url":"file:///etc/passwd"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a refused target, want 400", resp.StatusCode)
	}

	list := h.listAfter(t, h.do(t, http.MethodGet, "/_driftwood/api/projects", nil))
	if len(list.Projects) != 1 {
		t.Errorf("a project was created for a target that was refused: %d projects, want 1", len(list.Projects))
	}
}

func TestCreateProjectNeedsAName(t *testing.T) {
	h := newHarness(t)

	if resp := h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"  "}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a blank name, want 400", resp.StatusCode)
	}
}

// TestCreateProjectRefusesPastTheLimit exercises the bound on total memory.
// maxProjects is what keeps the per-project caps from multiplying without end,
// and the route has to surface its refusal rather than dropping the project.
func TestCreateProjectRefusesPastTheLimit(t *testing.T) {
	h := newHarness(t)

	created := 1 // the harness's own project
	for {
		resp := h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"filler"}`)
		if resp.StatusCode == http.StatusConflict {
			break
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d creating project %d, want 200 or 409", resp.StatusCode, created+1)
		}
		created++
		if created > 100 {
			t.Fatal("the project limit was never reached; there is no bound")
		}
	}

	if created < 2 {
		t.Fatalf("the limit was hit after %d projects, which leaves nothing to fill", created)
	}
}

// TestSetActiveProjectRouteSwitchesTheProxy is the integration the switcher
// depends on: the route changes the active project, and the proxy follows.
func TestSetActiveProjectRouteSwitchesTheProxy(t *testing.T) {
	h := newHarness(t)
	first := h.store.ActiveProject()

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)

	resp := h.postJSON(t, "/_driftwood/api/projects/active", `{"id":"`+created.ID+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := h.listAfter(t, resp).Active; got != created.ID {
		t.Errorf("active = %q, want %q", got, created.ID)
	}
	if h.store.ActiveProject() != created.ID {
		t.Errorf("the store's active project is %q, want %q", h.store.ActiveProject(), created.ID)
	}

	// The new project has no target, so a proxied request has nowhere to go and
	// must say so rather than borrowing the first project's backend.
	before := h.hitCount()
	if resp := h.do(t, http.MethodGet, "/api/users", nil); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d through an untargeted project, want 502", resp.StatusCode)
	}
	if h.hitCount() != before {
		t.Error("a request through an untargeted project reached another project's backend")
	}

	// Switching back restores the first project's target.
	if resp := h.postJSON(t, "/_driftwood/api/projects/active", `{"id":"`+first+`"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("switching back: status = %d", resp.StatusCode)
	}
	if resp := h.do(t, http.MethodGet, "/api/users", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d after switching back, want 200", resp.StatusCode)
	}
}

func TestSetActiveProjectRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	if resp := h.postJSON(t, "/_driftwood/api/projects/active", `{"id":"ghost"}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if h.store.ActiveProject() == "ghost" {
		t.Error("an unknown project became active")
	}
}

func TestUpdateProjectRenamesAndRetargets(t *testing.T) {
	h := newHarness(t)
	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Before"}`), &created)

	resp := h.postJSON(t, "/_driftwood/api/projects/update",
		`{"id":"`+created.ID+`","name":"After","target_url":"http://localhost:4343"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	projects := h.listAfter(t, resp).Projects

	var found bool
	for _, p := range projects {
		if p.ID != created.ID {
			continue
		}
		found = true
		if p.Name != "After" {
			t.Errorf("name = %q, want %q", p.Name, "After")
		}
		if p.TargetURL != "http://localhost:4343" {
			t.Errorf("target = %q, want the posted value", p.TargetURL)
		}
	}
	if !found {
		t.Fatalf("project %q is missing from the response", created.ID)
	}
}

// TestUpdateProjectOmittingTheTargetLeavesItAlone pins the pointer-versus-string
// distinction the request type is built around. A rename that wiped the target
// would leave a client pointing at nothing, and the dashboard's rename form does
// not send a target at all.
func TestUpdateProjectOmittingTheTargetLeavesItAlone(t *testing.T) {
	h := newHarness(t)
	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create",
		`{"name":"Before","target_url":"http://localhost:4242"}`), &created)

	if resp := h.postJSON(t, "/_driftwood/api/projects/update",
		`{"id":"`+created.ID+`","name":"Renamed"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	target, _, _ := h.store.ProjectTarget(created.ID)
	if target != "http://localhost:4242" {
		t.Errorf("target = %q after a rename that did not mention one, want it unchanged", target)
	}
}

func TestUpdateProjectRefusesAnInvalidTarget(t *testing.T) {
	h := newHarness(t)
	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create",
		`{"name":"Client B","target_url":"http://localhost:4242"}`), &created)

	// A scheme refusal, not a private address. The test client connects from
	// loopback, and a loopback caller is deliberately allowed to name a private
	// backend — that is what makes a local development install work, and the
	// refusal for private hosts applies to callers from off-box. A scheme is
	// refused whoever asks.
	resp := h.postJSON(t, "/_driftwood/api/projects/update",
		`{"id":"`+created.ID+`","target_url":"file:///etc/passwd"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a file:// target, want 400", resp.StatusCode)
	}
	if target, _, _ := h.store.ProjectTarget(created.ID); target != "http://localhost:4242" {
		t.Errorf("target = %q after a refused retarget, want it unchanged", target)
	}
}

func TestDeleteProjectRouteRefusesTheLastOne(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, "/_driftwood/api/projects/delete", `{"id":"`+h.store.ActiveProject()+`"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d deleting the last project, want 409", resp.StatusCode)
	}
	if projects, _ := h.store.ListProjects(); len(projects) != 1 {
		t.Error("the last project was deleted")
	}
}

func TestDeleteProjectRouteRemovesItAndReassignsActive(t *testing.T) {
	h := newHarness(t)
	first := h.store.ActiveProject()

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/active", `{"id":"`+created.ID+`"}`), &struct{}{})

	resp := h.postJSON(t, "/_driftwood/api/projects/delete", `{"id":"`+created.ID+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	list := h.listAfter(t, resp)
	if len(list.Projects) != 1 {
		t.Errorf("got %d projects after deleting one of two, want 1", len(list.Projects))
	}
	// Deleting the project that held the active flag has to move it, or the
	// install is left with an active project that does not exist.
	if list.Active != first {
		t.Errorf("active = %q after deleting the active project, want the remaining one %q", list.Active, first)
	}
	if h.store.ProjectExists(created.ID) {
		t.Errorf("project %q still exists", created.ID)
	}
}

func TestDeleteProjectRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	if resp := h.postJSON(t, "/_driftwood/api/projects/delete", `{"id":"ghost"}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestProjectScopedReadsFollowTheParameter covers ?project= on the read routes:
// an explicit project is read, and the absent case reads the active one.
func TestProjectScopedReadsFollowTheParameter(t *testing.T) {
	h := newHarness(t)

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)
	active := h.store.ActiveProject()

	// Traffic under each project, recorded through the store so this test is
	// about the read routes rather than about how traffic is raised.
	h.store.AddTraffic(active, storageTestTraffic(active, "/first"))
	h.store.AddTraffic(created.ID, storageTestTraffic(created.ID, "/second"))

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/_driftwood/api/traffic", "/first"},
		{"/_driftwood/api/traffic?project=" + created.ID, "/second"},
	} {
		resp := h.do(t, http.MethodGet, tc.path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", tc.path, resp.StatusCode)
		}
		var traffic []struct {
			Path string `json:"path"`
		}
		decodeBody(t, resp, &traffic)
		if len(traffic) != 1 {
			t.Errorf("GET %s: got %d records, want exactly its own project's one", tc.path, len(traffic))
			continue
		}
		if traffic[0].Path != tc.want {
			t.Errorf("GET %s: got %q, want %q", tc.path, traffic[0].Path, tc.want)
		}
	}
}

// TestProjectScopedReadRefusesAnUnknownProject is the anti-fallback assertion.
// Answering a question about a project that does not exist with the active
// project's data would be the quiet misattribution this codebase is strict
// about, and a typo in a script would silently read the wrong client's
// contracts.
func TestProjectScopedReadRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{
		"/_driftwood/api/traffic?project=ghost",
		"/_driftwood/api/alerts?project=ghost",
		"/_driftwood/api/baselines?project=ghost",
		"/_driftwood/api/histories?project=ghost",
		"/_driftwood/api/export/typescript?project=ghost",
	} {
		resp := h.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
	}
}

// TestProjectWritesHonourTheParameter is the other half of the uniform rule.
// A write that named a project and landed on the active one would leave a caller
// reading one client and writing another.
func TestProjectWritesHonourTheParameter(t *testing.T) {
	h := newHarness(t)
	active := h.store.ActiveProject()

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)

	resp := h.postJSON(t, "/_driftwood/api/baselines?project="+created.ID,
		`{"method":"GET","path":"/orders","payload":"{\"id\":1}"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if _, ok := h.store.GetBaseline(created.ID, "GET", "/orders"); !ok {
		t.Error("the baseline was not recorded against the project the request named")
	}
	if _, ok := h.store.GetBaseline(active, "GET", "/orders"); ok {
		t.Error("the baseline landed on the active project as well as the named one")
	}
}

// TestSubscribeScopesTheStream covers ?project= on the events route, which is
// what makes the dashboard's live view follow its switcher.
func TestSubscribeScopesTheStream(t *testing.T) {
	h := newHarness(t)
	active := h.store.ActiveProject()

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)

	resp, err := http.Get(h.router.URL + "/_driftwood/events?project=" + created.ID)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want an event stream", ct)
	}

	frames := make(chan map[string]interface{}, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) == nil && frame["type"] != nil {
				frames <- frame
			}
		}
	}()

	// The subscriber is registered by the time the ping has been written, so a
	// short wait here is about the ping reaching us, not about the hub.
	time.Sleep(100 * time.Millisecond)

	h.hub.Publish(active, "traffic", map[string]interface{}{"path": "/not-yours"})
	h.hub.Publish(created.ID, "traffic", map[string]interface{}{"path": "/yours"})

	select {
	case frame := <-frames:
		data, _ := frame["data"].(map[string]interface{})
		if data["path"] != "/yours" {
			t.Errorf("the stream carried %v; a subscriber on %s received %s's event", data["path"], created.ID, active)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing arrived on the stream")
	}
}

func TestSubscribeRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, "/_driftwood/events?project=ghost", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d subscribing to a project that does not exist, want 404 — a stream that "+
			"carries nothing looks identical to a project with no traffic", resp.StatusCode)
	}
}

// TestProjectRoutesAnnounceTheChange covers the event the switcher listens for.
//
// A dashboard that is not the one making the change has no other way to learn
// about it: without this frame, a project created in one tab is invisible in
// another until a reload, and a project deleted by someone else leaves a
// switcher offering a project that no longer exists.
func TestProjectRoutesAnnounceTheChange(t *testing.T) {
	h := newHarness(t)
	active := h.store.ActiveProject()

	resp, err := http.Get(h.router.URL + "/_driftwood/events?project=" + active)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()

	frames := make(chan map[string]interface{}, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) == nil && frame["type"] != nil {
				frames <- frame
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)

	var created struct {
		ID string `json:"id"`
	}
	decodeBody(t, h.postJSON(t, "/_driftwood/api/projects/create", `{"name":"Client B"}`), &created)

	select {
	case frame := <-frames:
		if frame["type"] != "projects_updated" {
			t.Fatalf("frame type = %v, want projects_updated", frame["type"])
		}
		data, _ := frame["data"].(map[string]interface{})
		projects, _ := data["projects"].([]interface{})
		if len(projects) != 2 {
			t.Errorf("the announced list holds %d projects, want the 2 that exist", len(projects))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("creating a project announced nothing; another dashboard would never learn about it")
	}
}

// The setup wizard asks "where is your API running?", so it has to open when
// that question is unanswered for the project in force — not merely when nobody
// has ever saved a setting.
//
// The distinction matters because IsConfigured is set once and never unset, so
// it reports on the install rather than on the project. An install launched
// with --target is "configured" forever, which meant that after a second
// project was created without a backend and switched to, every surface that
// could ask for a target was switched off: the wizard by this flag, and startup
// by a log.Fatalf that took the whole install down instead. That is the pair of
// holes this test and the one below it close.
func TestSetupStateAsksWhenTheActiveProjectHasNoTarget(t *testing.T) {
	h := newHarness(t)

	// The deploy box's state: launched once with --target, so the install counts
	// as configured and keeps counting that way.
	h.store.SetConfigured(true)

	project, err := h.store.CreateProject("Acme")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := h.store.SetActiveProject(project.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	state := func() bool {
		resp := h.do(t, http.MethodGet, "/_driftwood/api/setup-state", nil)
		var body struct {
			Configured bool `json:"configured"`
		}
		decodeBody(t, resp, &body)
		return body.Configured
	}

	if state() {
		t.Error("a project with no target reported the install as configured, so nothing asks where its API is")
	}

	// And the other direction, so this cannot be satisfied by always asking: a
	// target on the project answers the question and closes the wizard.
	if err := h.store.SetProjectTarget(project.ID, "http://example.com", false); err != nil {
		t.Fatalf("SetProjectTarget: %v", err)
	}
	if !state() {
		t.Error("a project with a target still reported the install as unconfigured")
	}
}
