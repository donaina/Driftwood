package storage

import (
	"strings"
	"testing"
)

// These four cases used to live beside the command, because the command owned
// the function. It moved here when the server route that imports over the API
// needed the same answer to "which project", so the tests moved with it — the
// alternative is a rule enforced in two places, which is the disagreement the
// function exists to prevent.

// Without -project an import must land exactly where it landed before projects
// existed. This is the case that keeps the flag additive.
func TestResolveImportProjectDefaultsToActive(t *testing.T) {
	store := testStore(t)
	first := store.ActiveProject()

	got, err := ResolveImportProject(store, "")
	if err != nil {
		t.Fatalf("ResolveImportProject(\"\"): %v", err)
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

	got, err = ResolveImportProject(store, "")
	if err != nil {
		t.Fatalf("ResolveImportProject(\"\"): %v", err)
	}
	if got != second.ID {
		t.Errorf("an unflagged import resolved to %q after switching, want %q", got, second.ID)
	}
}

func TestResolveImportProjectHonoursAnExplicitProject(t *testing.T) {
	store := testStore(t)
	other, err := store.CreateProject("Acme")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := ResolveImportProject(store, other.ID)
	if err != nil {
		t.Fatalf("ResolveImportProject(%q): %v", other.ID, err)
	}
	if got != other.ID {
		t.Errorf("resolved to %q, want %q", got, other.ID)
	}
}

// A named project that does not exist is refused. Creating it instead would
// file a client's contracts under a project the user never made and nothing is
// pointed at, and the import would report success.
func TestResolveImportProjectRefusesAnUnknownProject(t *testing.T) {
	store := testStore(t)
	before, _ := store.ListProjects()

	got, err := ResolveImportProject(store, "acme")
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

// A project whose name differs from its id is offered with both, because the
// flag takes the id and the name is how a person knows which project it is.
func TestResolveImportProjectNamesKnownProjectsWithTheirNames(t *testing.T) {
	store := testStore(t)
	if _, err := store.CreateProject("Acme Widgets"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	_, err := ResolveImportProject(store, "acme-widget")
	if err == nil {
		t.Fatal("resolved a project id that does not exist, want an error")
	}
	if !strings.Contains(err.Error(), "Acme Widgets") {
		t.Errorf("the error omits the known project's name, leaving the user to guess which id it is: %v", err)
	}
}
