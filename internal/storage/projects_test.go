package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

// secondProject creates a project beside the default one and returns its id.
//
// The active project is deliberately left where it was. Every test below names
// the project it means, and a helper that also switched would hide the one thing
// these tests exist to check — that naming a project is what decides where data
// goes, not which project happens to be on screen.
func secondProject(t *testing.T, s *Store, name string) string {
	t.Helper()
	p, err := s.CreateProject(name)
	if err != nil {
		t.Fatalf("CreateProject(%q): %v", name, err)
	}
	return p.ID
}

func alertingTraffic(id, method, path string) types.CapturedTraffic {
	return types.CapturedTraffic{
		ID:             id,
		Method:         method,
		Path:           path,
		Timestamp:      time.Now(),
		StatusCode:     200,
		ContractStatus: "BREAKING",
		Diff: &types.ContractDiff{
			HasBreakingChanges: true,
			Deltas: []types.DiffDelta{
				{Severity: types.SeverityBreaking, Kind: types.KindRemovedField, JSONPath: "$.id"},
			},
		},
	}
}

// TestProjectsIsolateTheSameEndpoint is the test this whole step exists for.
//
// Both projects are given the same METHOD:PATH with different payloads, which is
// the case a project-prefixed key would have to get right and the case every
// other isolation test would pass by accident if the key were merely namespaced
// somewhere that one of the four stores forgot to consult.
//
// Baselines, histories, traffic and alerts are each asserted, because they are
// four separate maps and getting three of them right is the likely failure.
func TestProjectsIsolateTheSameEndpoint(t *testing.T) {
	s := testStore(t)
	alpha := s.ActiveProject()
	beta := secondProject(t, s, "Beta")

	if _, err := s.SaveBaseline(alpha, "GET", "/api/users", `{"project": "alpha"}`); err != nil {
		t.Fatalf("saving alpha's baseline: %v", err)
	}
	if _, err := s.SaveBaseline(beta, "GET", "/api/users", `{"project": "beta"}`); err != nil {
		t.Fatalf("saving beta's baseline: %v", err)
	}

	// Baselines: each project reads back its own payload under the same key.
	for _, tc := range []struct{ project, want string }{
		{alpha, `{"project": "alpha"}`},
		{beta, `{"project": "beta"}`},
	} {
		cb, ok := s.GetBaseline(tc.project, "GET", "/api/users")
		if !ok {
			t.Fatalf("project %q has no baseline for the endpoint it saved", tc.project)
		}
		if cb.SamplePayload != tc.want {
			t.Errorf("project %q reads payload %s, want %s — the key is shared across projects",
				tc.project, cb.SamplePayload, tc.want)
		}
	}

	// The list views too: one baseline each, not two.
	for _, project := range []string{alpha, beta} {
		if got := len(s.GetAllBaselines(project)); got != 1 {
			t.Errorf("project %q lists %d baselines, want 1", project, got)
		}
		if got := len(s.GetAllHistories(project)); got != 1 {
			t.Errorf("project %q lists %d histories, want 1", project, got)
		}
	}

	// Traffic.
	s.AddTraffic(alpha, alertingTraffic("tr_alpha", "GET", "/api/users"))
	s.AddTraffic(beta, alertingTraffic("tr_beta", "GET", "/api/users"))

	for project, wantID := range map[string]string{alpha: "tr_alpha", beta: "tr_beta"} {
		got := s.GetTraffics(project, 100)
		if len(got) != 1 {
			t.Fatalf("project %q holds %d traffic records, want 1", project, len(got))
		}
		if got[0].ID != wantID {
			t.Errorf("project %q holds traffic %q, want %q", project, got[0].ID, wantID)
		}
		if got[0].ProjectID != project {
			t.Errorf("traffic %q is stamped with project %q, want %q",
				got[0].ID, got[0].ProjectID, project)
		}
	}

	// Alerts. These are the ones that would leak silently: the alert map is keyed
	// by traffic ID and holds no project, so the only thing keeping them apart is
	// alertOrder.
	for project, wantID := range map[string]string{alpha: "tr_alpha", beta: "tr_beta"} {
		got := s.GetAlerts(project, 100)
		if len(got) != 1 {
			t.Fatalf("project %q lists %d alerts, want 1", project, len(got))
		}
		if got[0].TrafficID != wantID {
			t.Errorf("project %q lists alert %q, want %q", project, got[0].TrafficID, wantID)
		}
	}
}

// TestTrafficIsStampedWithItsProject pins the record's own account of where it
// came from, which is the field the traffic view reads instead of inferring the
// project from which list the record was found in.
func TestTrafficIsStampedWithItsProject(t *testing.T) {
	s := testStore(t)
	project := secondProject(t, s, "Stamped")

	// Stamped by the store, so a caller that mislabels its own record cannot
	// mislabel the store's.
	s.AddTraffic(project, types.CapturedTraffic{
		ID: "tr_1", Method: "GET", Path: "/api/x", ProjectID: "somewhere-else",
	})

	got := s.GetTraffics(project, 10)
	if len(got) != 1 {
		t.Fatalf("got %d traffic records, want 1", len(got))
	}
	if got[0].ProjectID != project {
		t.Errorf("traffic is stamped %q, want the project it was filed under (%q)",
			got[0].ProjectID, project)
	}
}

// TestProjectCapsArePerProject is the other half of isolation, and the half a
// project field on a globally-capped map would pass: filling one project past
// the endpoint cap must not evict anything from another.
//
// This is what makes the cap a per-project fact rather than a global one. One
// busy client evicting another client's contracts is not isolation with a rough
// edge — the eviction is driven by traffic the other project never sent.
func TestProjectCapsArePerProject(t *testing.T) {
	s := testStore(t)
	quiet := s.ActiveProject()
	busy := secondProject(t, s, "Busy")

	// One endpoint in the quiet project that must survive.
	if _, err := s.SaveBaseline(quiet, "GET", "/api/quiet", `{"q": 1}`); err != nil {
		t.Fatalf("saving the quiet project's baseline: %v", err)
	}

	// Fill the busy project past the cap. None of these are pinned or confirmed,
	// so all of them are eligible for eviction and the cap is actually exercised.
	for i := 0; i < maxEndpoints+50; i++ {
		s.AddTraffic(busy, types.CapturedTraffic{
			ID: fmt.Sprintf("tr_%d", i), Method: "GET",
			Path: fmt.Sprintf("/api/busy/%d", i), Timestamp: time.Now(),
		})
	}

	if got := len(s.histories[busy]); got > maxEndpoints {
		t.Errorf("the busy project holds %d endpoints, above the cap of %d", got, maxEndpoints)
	}
	if got := len(s.histories[busy]); got < maxEndpoints {
		t.Errorf("the busy project holds %d endpoints, want the cap of %d reached — "+
			"the test did not actually exercise the cap", got, maxEndpoints)
	}
	if _, ok := s.GetBaseline(quiet, "GET", "/api/quiet"); !ok {
		t.Error("the other project's endpoint was evicted by traffic it never sent, " +
			"so the cap is not per project")
	}
}

// TestProjectCRUD covers the operations the switcher and the settings view are
// built on, including the two refusals: an unnamed project, and deleting the
// last one.
func TestProjectCRUD(t *testing.T) {
	s := testStore(t)
	first := s.ActiveProject()

	if _, err := s.CreateProject("   "); err == nil {
		t.Error("a project with a blank name was accepted")
	}

	p, err := s.CreateProject("Acme Ltd")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// The id is derived and usable in a URL; the name is kept for display.
	if p.ID != "acme-ltd" {
		t.Errorf("id = %q, want the slug %q", p.ID, "acme-ltd")
	}
	if p.Name != "Acme Ltd" {
		t.Errorf("name = %q, want %q", p.Name, "Acme Ltd")
	}

	// A second project of the same name gets a distinct id rather than
	// displacing the first, which is what makes creating one safe.
	dup, err := s.CreateProject("Acme Ltd")
	if err != nil {
		t.Fatalf("CreateProject (duplicate name): %v", err)
	}
	if dup.ID == p.ID {
		t.Fatalf("a second project took the id %q of an existing one", dup.ID)
	}

	list, active := s.ListProjects()
	if len(list) != 3 {
		t.Fatalf("ListProjects returned %d projects, want 3", len(list))
	}
	if active != first {
		t.Errorf("active = %q, want %q — creating a project must not switch to it", active, first)
	}
	if list[0].ID != first {
		t.Errorf("projects are not in creation order: first is %q, want %q", list[0].ID, first)
	}

	// Renaming changes the display name and not the id: the id is in the
	// document and in any URL a user has kept.
	if err := s.RenameProject(p.ID, "Acme Holdings"); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	list, _ = s.ListProjects()
	for _, got := range list {
		if got.ID != p.ID {
			continue
		}
		if got.Name != "Acme Holdings" {
			t.Errorf("name after rename = %q, want %q", got.Name, "Acme Holdings")
		}
	}
	if err := s.RenameProject(p.ID, "  "); err == nil {
		t.Error("renaming a project to a blank name was accepted")
	}
	if err := s.RenameProject("no-such-project", "x"); err == nil {
		t.Error("renaming a project that does not exist was accepted")
	}

	// Switching.
	if err := s.SetActiveProject(dup.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}
	if _, active := s.ListProjects(); active != dup.ID {
		t.Errorf("active = %q, want %q", active, dup.ID)
	}
	if err := s.SetActiveProject("no-such-project"); err == nil {
		t.Error("switching to a project that does not exist was accepted")
	}

	// Deleting. The active project is the one being removed, so the store has to
	// hand the active role to something else rather than leave it dangling.
	if err := s.DeleteProject(dup.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	list, active = s.ListProjects()
	if len(list) != 2 {
		t.Errorf("after deleting one of three, %d projects remain, want 2", len(list))
	}
	if active == dup.ID {
		t.Error("the deleted project is still the active one")
	}
	if !s.ProjectExists(active) {
		t.Errorf("active project %q is not one of the surviving projects", active)
	}
	if err := s.DeleteProject(dup.ID); err == nil {
		t.Error("deleting a project that does not exist was accepted")
	}
}

// TestDeleteProjectRefusesTheLastOne is the refusal that keeps the rest of the
// store's assumptions true: every accessor is written against a project
// existing, so an install with none is a state nothing here describes.
func TestDeleteProjectRefusesTheLastOne(t *testing.T) {
	s := testStore(t)
	only := s.ActiveProject()

	err := s.DeleteProject(only)
	if err == nil {
		t.Fatal("the last project was deleted, leaving an install with no active project")
	}
	if !s.ProjectExists(only) {
		t.Error("the refusal removed the project anyway")
	}
	if _, active := s.ListProjects(); active != only {
		t.Errorf("active = %q after a refused delete, want %q", active, only)
	}
}

// TestDeleteProjectCollectsItsAlerts covers the one dimension the delete has to
// reconstruct by hand. Alerts are keyed by traffic ID rather than by project, so
// nothing else would ever remove a deleted project's alerts: the map would keep
// one entry per alert the project ever raised, reachable from nowhere, for the
// life of the process.
func TestDeleteProjectCollectsItsAlerts(t *testing.T) {
	s := testStore(t)
	doomed := secondProject(t, s, "Doomed")
	survivor := s.ActiveProject()

	s.AddTraffic(doomed, alertingTraffic("tr_doomed", "GET", "/api/doomed"))
	s.AddTraffic(survivor, alertingTraffic("tr_survivor", "GET", "/api/survivor"))

	if _, ok := s.alerts["tr_doomed"]; !ok {
		t.Fatal("the alert was never raised, so this test proves nothing")
	}

	if err := s.DeleteProject(doomed); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, ok := s.alerts["tr_doomed"]; ok {
		t.Error("the deleted project's alert is still in the alert map, reachable from nowhere")
	}
	if _, ok := s.alertOrder[doomed]; ok {
		t.Error("the deleted project still has an alert-order entry")
	}
	// And the survivor's is untouched.
	if _, ok := s.alerts["tr_survivor"]; !ok {
		t.Error("deleting one project removed another project's alert")
	}
	if got := s.GetAlerts(survivor, 10); len(got) != 1 {
		t.Errorf("the surviving project lists %d alerts, want 1", len(got))
	}
}

// TestUnknownProjectIsNotConjuredIntoBeing guards the line between "this
// project has no data" and "there is no such project".
//
// The writes are the half that matters, and this test drove only the reads
// until a mutation showed the reads cannot fail it: GetBaseline and GetHistory
// index the map directly and never reach ensureProjectLocked, so no change to
// that function could be caught from here. AddTraffic, ClearTraffic and
// SaveBaselineWithSchema are the paths that do call it — they have to, because
// they allocate — so they are where "it does not also register the project" is
// decidable.
//
// What it is protecting: a project id can arrive from a query parameter, and a
// typo there must not silently create a project. The switcher would grow an
// entry nobody made, and it would be one the operator cannot explain.
func TestUnknownProjectIsNotConjuredIntoBeing(t *testing.T) {
	s := testStore(t)
	before, _ := s.ListProjects()

	const ghost = "no-such-project"

	// Reads answer nothing.
	if _, ok := s.GetBaseline(ghost, "GET", "/api/users"); ok {
		t.Error("a baseline was found for a project that does not exist")
	}
	if _, ok := s.GetHistory(ghost, "GET", "/api/users"); ok {
		t.Error("a history was found for a project that does not exist")
	}
	if got := s.GetTraffics(ghost, 10); len(got) != 0 {
		t.Errorf("a project that does not exist holds %d traffic records", len(got))
	}
	if got := s.GetAlerts(ghost, 10); len(got) != 0 {
		t.Errorf("a project that does not exist holds %d alerts", len(got))
	}
	if got := s.GetAllBaselines(ghost); len(got) != 0 {
		t.Errorf("a project that does not exist holds %d baselines", len(got))
	}

	// Writes to it go nowhere that is reachable, and do not make it real.
	s.AddTraffic(ghost, alertingTraffic("tr_ghost", "GET", "/api/users"))
	if _, err := s.SaveBaseline(ghost, "GET", "/api/users", `{"ghost": true}`); err != nil {
		t.Fatalf("saving into a project that does not exist: %v", err)
	}
	s.ClearTraffic(ghost)

	if s.ProjectExists(ghost) {
		t.Error("writing to a project that does not exist created it")
	}
	after, active := s.ListProjects()
	if len(after) != len(before) {
		t.Errorf("the project list grew from %d to %d entries without a project being created",
			len(before), len(after))
	}
	if active != s.ActiveProject() {
		t.Errorf("active project moved to %q as a side effect of writing to %q",
			active, ghost)
	}
}

// TestProjectsSurviveARestart is the round trip across the document: two
// projects, each with an endpoint of the same name and a different contract, and
// the active one remembered.
func TestProjectsSurviveARestart(t *testing.T) {
	s := testStore(t)
	alpha := s.ActiveProject()
	beta := secondProject(t, s, "Beta")

	if _, err := s.SaveBaseline(alpha, "GET", "/api/users", `{"from": "alpha"}`); err != nil {
		t.Fatalf("alpha's baseline: %v", err)
	}
	if _, err := s.SaveBaseline(beta, "GET", "/api/users", `{"from": "beta"}`); err != nil {
		t.Fatalf("beta's baseline: %v", err)
	}
	s.AddTraffic(beta, alertingTraffic("tr_beta", "GET", "/api/users"))

	// Switch to beta, so the restart also has to remember which one was on screen.
	if err := s.SetActiveProject(beta); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	restarted := reopenStore(t, s)
	if err := restarted.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}

	if _, active := restarted.ListProjects(); active != beta {
		t.Errorf("active after a restart = %q, want the remembered %q", active, beta)
	}
	projects, _ := restarted.ListProjects()
	if len(projects) != 2 {
		t.Fatalf("after a restart there are %d projects, want 2", len(projects))
	}

	for _, tc := range []struct{ project, want string }{
		{alpha, `{"from": "alpha"}`},
		{beta, `{"from": "beta"}`},
	} {
		cb, ok := restarted.GetBaseline(tc.project, "GET", "/api/users")
		if !ok {
			t.Errorf("project %q lost its baseline across a restart", tc.project)
			continue
		}
		if cb.SamplePayload != tc.want {
			t.Errorf("project %q reads %s after a restart, want %s",
				tc.project, cb.SamplePayload, tc.want)
		}
	}
}

// TestAlertsAreNotPersisted pins a boundary this test found rather than assumed:
// the document holds projects and their contracts, and nothing else. The traffic
// ring and the alert list are live views of what has come through the proxy since
// it started.
//
// That is coherent rather than an omission. An alert is a notification about a
// response, and it is keyed by the traffic ID of that response; the response
// itself is not kept either. Persisting the alert alone would leave a record
// pointing at a traffic entry that no longer exists, so the alert view would
// offer a link to a request nobody can open.
//
// Written down because the opposite is the reasonable guess — an alert feels
// like something you would want to still have in the morning — so the next
// reader should find the decision stated, with a test holding it, rather than
// discover it by writing a test like the one this replaced.
func TestAlertsAreNotPersisted(t *testing.T) {
	s := testStore(t)
	project := s.ActiveProject()

	s.AddTraffic(project, alertingTraffic("tr_1", "GET", "/api/users"))
	if got := s.GetAlerts(project, 10); len(got) != 1 {
		t.Fatalf("the alert was never raised, so this test proves nothing: got %d", len(got))
	}

	restarted := reopenStore(t, s)
	if err := restarted.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}

	if got := restarted.GetAlerts(project, 10); len(got) != 0 {
		t.Errorf("a restart surfaced %d alerts; alerts are in-memory, so the document "+
			"has grown an alerts field this test does not know about", len(got))
	}
	// The same is true of the traffic an alert refers to, and the two go
	// together: an alert is only meaningful beside the request that raised it.
	if got := restarted.GetTraffics(project, 10); len(got) != 0 {
		t.Errorf("a restart surfaced %d traffic records, want 0", len(got))
	}
}

// TestCreateProjectAtTheLimitIsRefused pins the install-wide bound. Every other
// cap here is per project, so without this one the total is bounded only by how
// many times a person clicks "new project".
func TestCreateProjectAtTheLimitIsRefused(t *testing.T) {
	s := testStore(t)

	// The store seeds one, so this brings the count to exactly maxProjects.
	for i := 1; i < maxProjects; i++ {
		if _, err := s.CreateProject(fmt.Sprintf("Client %d", i)); err != nil {
			t.Fatalf("creating project %d of %d: %v", i+1, maxProjects, err)
		}
	}
	if list, _ := s.ListProjects(); len(list) != maxProjects {
		t.Fatalf("holding %d projects, want the limit of %d", len(list), maxProjects)
	}

	if _, err := s.CreateProject("One Too Many"); err == nil {
		t.Errorf("a %dth project was accepted despite the limit of %d", maxProjects+1, maxProjects)
	}
}

// TestDeletingAProjectFreesItsSlot checks the refusal above is a limit and not a
// dead end: the way to make room is to remove something.
func TestDeletingAProjectFreesItsSlot(t *testing.T) {
	s := testStore(t)
	for i := 1; i < maxProjects; i++ {
		if _, err := s.CreateProject(fmt.Sprintf("Client %d", i)); err != nil {
			t.Fatalf("filling up: %v", err)
		}
	}

	list, _ := s.ListProjects()
	last := list[len(list)-1].ID
	if err := s.DeleteProject(last); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.CreateProject("The Replacement"); err != nil {
		t.Errorf("a slot freed by deleting a project could not be reused: %v", err)
	}
}

// TestDeletedProjectDataIsGoneCompletely checks nothing is left behind that a
// later read could surface. The maps are separate, so a delete that removed the
// project from the list and forgot one of them would leave the data reachable
// through an accessor that never consults the project list.
func TestDeletedProjectDataIsGoneCompletely(t *testing.T) {
	s := testStore(t)
	doomed := secondProject(t, s, "Doomed")

	if _, err := s.SaveBaseline(doomed, "GET", "/api/users", `{"gone": true}`); err != nil {
		t.Fatalf("saving: %v", err)
	}
	s.AddTraffic(doomed, alertingTraffic("tr_1", "GET", "/api/users"))

	if err := s.DeleteProject(doomed); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, ok := s.GetBaseline(doomed, "GET", "/api/users"); ok {
		t.Error("a baseline survived its project's deletion")
	}
	if got := s.GetTraffics(doomed, 10); len(got) != 0 {
		t.Errorf("%d traffic records survived their project's deletion", len(got))
	}
	if got := s.GetAlerts(doomed, 10); len(got) != 0 {
		t.Errorf("%d alerts survived their project's deletion", len(got))
	}
}
