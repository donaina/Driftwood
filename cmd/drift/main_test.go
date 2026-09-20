package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// Without -project an import must land exactly where it landed before projects
// existed. This is the case that keeps the flag additive.
func TestResolveImportProjectDefaultsToActive(t *testing.T) {
	store := newTestStore(t)
	first := store.ActiveProject()

	got, err := resolveImportProject(store, "")
	if err != nil {
		t.Fatalf("resolveImportProject(\"\"): %v", err)
	}
	if got != first {
		t.Errorf("an unflagged import resolved to %q, want the active project %q", got, first)
	}

	// And it follows the active project rather than being pinned to the
	// default, which is the difference between "the active project" and "the
	// project that happened to be active when this code was written".
	second, err := store.CreateProject("Second")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := store.SetActiveProject(second.ID); err != nil {
		t.Fatalf("SetActiveProject: %v", err)
	}

	got, err = resolveImportProject(store, "")
	if err != nil {
		t.Fatalf("resolveImportProject(\"\"): %v", err)
	}
	if got != second.ID {
		t.Errorf("an unflagged import resolved to %q after switching, want %q", got, second.ID)
	}
}

func TestResolveImportProjectHonoursAnExplicitProject(t *testing.T) {
	store := newTestStore(t)
	other, err := store.CreateProject("Acme")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := resolveImportProject(store, other.ID)
	if err != nil {
		t.Fatalf("resolveImportProject(%q): %v", other.ID, err)
	}
	if got != other.ID {
		t.Errorf("resolved to %q, want %q", got, other.ID)
	}
}

// A named project that does not exist is refused. Creating it instead would
// file a client's contracts under a project the user never made and nothing is
// pointed at, and the import would report success.
func TestResolveImportProjectRefusesAnUnknownProject(t *testing.T) {
	store := newTestStore(t)
	before, _ := store.ListProjects()

	got, err := resolveImportProject(store, "acme")
	if err == nil {
		t.Fatalf("resolved %q to %q, want an error", "acme", got)
	}

	// The error has to name what does exist, or the user's only recourse is to
	// guess at the ids.
	active := store.ActiveProject()
	if !strings.Contains(err.Error(), active) {
		t.Errorf("the error does not name the project that does exist (%q): %v", active, err)
	}

	after, _ := store.ListProjects()
	if len(after) != len(before) {
		t.Errorf("a refused import changed the project list: %d -> %d", len(before), len(after))
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
