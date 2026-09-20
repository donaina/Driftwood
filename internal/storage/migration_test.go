package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

// driftwoodHome points HOME at a temp directory and returns the path the store
// will use, so a test can put a file there before the store reads it.
//
// Every test in this file goes through NewStore rather than a field literal,
// because the thing under test is what NewStore does with a file it finds on
// disk. A hand-built Store has no file and no load, which is precisely the part
// these tests exist to cover.
func driftwoodHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".driftwood")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	return dir
}

// writeV1 writes a store in the pre-version format: a bare map of endpoints,
// with no wrapping document. Built from the real types so the fixture is the
// shape the old code actually wrote, not a hand-typed approximation of it.
func writeV1(t *testing.T, path string, histories map[string]*types.EndpointHistory) []byte {
	t.Helper()

	data, err := json.MarshalIndent(histories, "", "  ")
	if err != nil {
		t.Fatalf("building the v1 fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing the v1 fixture: %v", err)
	}
	return data
}

// v1History is one endpoint as the old format stored it.
func v1History(method, path, payload string, at time.Time) *types.EndpointHistory {
	return &types.EndpointHistory{
		Method: method,
		Path:   path,
		Versions: []*types.ContractBaseline{{
			ID:            "b1",
			Method:        method,
			Path:          path,
			SamplePayload: payload,
			CreatedAt:     at,
			UpdatedAt:     at,
			Version:       1,
			Source:        types.BaselineSourceManual,
		}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func TestFreshInstallWritesAStoreDocumentWithOneProject(t *testing.T) {
	dir := driftwoodHome(t)

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("a fresh install reported an error: %v", err)
	}
	if got := store.ActiveProject(); got != defaultProjectID {
		t.Errorf("active project on a fresh install = %q, want %q", got, defaultProjectID)
	}

	if _, err := store.SaveBaseline(store.ActiveProject(), "GET", "/api/users", `{"id": 1}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "baselines.json"))
	if err != nil {
		t.Fatalf("a save wrote no document: %v", err)
	}
	var doc persistedState
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the document a save wrote is unreadable: %v", err)
	}
	if doc.Version != storeVersion {
		t.Errorf("document version = %d, want %d", doc.Version, storeVersion)
	}
	if doc.Active != defaultProjectID {
		t.Errorf("document active project = %q, want %q", doc.Active, defaultProjectID)
	}
	if len(doc.Projects) != 1 {
		t.Fatalf("document lists %d projects, want 1", len(doc.Projects))
	}
	if doc.Histories[defaultProjectID]["GET:/api/users"] == nil {
		t.Error("the saved endpoint is not under the active project in the document")
	}
}

func TestV1StoreMigratesWithoutLosingAContract(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	now := time.Now().Truncate(time.Second)
	original := writeV1(t, path, map[string]*types.EndpointHistory{
		"GET:/api/users":       v1History("GET", "/api/users", `{"id": 1, "name": "Alice"}`, now),
		"POST:/api/orders":     v1History("POST", "/api/orders", `{"total": 10}`, now),
		"DELETE:/api/sessions": v1History("DELETE", "/api/sessions", `{}`, now),
	})

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("migrating a v1 store reported an error: %v", err)
	}

	// Every endpoint, not just the first: a migration that keeps one endpoint
	// and drops the rest is the failure that matters here.
	for _, endpoint := range []struct{ method, path, payload string }{
		{"GET", "/api/users", `{"id": 1, "name": "Alice"}`},
		{"POST", "/api/orders", `{"total": 10}`},
		{"DELETE", "/api/sessions", `{}`},
	} {
		cb, ok := store.GetBaseline(store.ActiveProject(), endpoint.method, endpoint.path)
		if !ok {
			t.Errorf("%s %s did not survive the migration", endpoint.method, endpoint.path)
			continue
		}
		if cb.SamplePayload != endpoint.payload {
			t.Errorf("%s %s payload = %s, want %s",
				endpoint.method, endpoint.path, cb.SamplePayload, endpoint.payload)
		}
		if cb.Source != types.BaselineSourceManual {
			t.Errorf("%s %s source = %q, want it preserved as %q",
				endpoint.method, endpoint.path, cb.Source, types.BaselineSourceManual)
		}
	}

	// The old file is kept, byte for byte, under its own name. Compared against
	// the fixture rather than merely checked for existence: a backup that was
	// written from the already-converted state would still be a file.
	backup, err := os.ReadFile(path + ".v1")
	if err != nil {
		t.Fatalf("the previous store was not kept: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Error("the kept copy is not the file that was there before the migration")
	}

	// And baselines.json itself is now the current format, so the next start
	// does not migrate again.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the converted store: %v", err)
	}
	var doc persistedState
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the converted store is unreadable: %v", err)
	}
	if doc.Version != storeVersion {
		t.Errorf("converted store version = %d, want %d", doc.Version, storeVersion)
	}
	if doc.Histories[defaultProjectID]["GET:/api/users"] == nil {
		t.Error("the converted store does not hold the migrated endpoint")
	}
}

func TestV2StoreIsReadAndNotRewritten(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	now := time.Now().Truncate(time.Second)
	doc := persistedState{
		Version:  storeVersion,
		Active:   "acme",
		Projects: []types.Project{{ID: "acme", Name: "Acme", CreatedAt: now}},
		Histories: map[string]map[string]*types.EndpointHistory{
			"acme": {"GET:/api/users": v1History("GET", "/api/users", `{"id": 7}`, now)},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("building the v2 fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing the v2 fixture: %v", err)
	}

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("reading a current store reported an error: %v", err)
	}
	if got := store.ActiveProject(); got != "acme" {
		t.Errorf("active project = %q, want the one the document named", got)
	}
	if cb, ok := store.GetBaseline(store.ActiveProject(), "GET", "/api/users"); !ok || cb.SamplePayload != `{"id": 7}` {
		t.Error("the endpoint recorded in the document was not loaded")
	}

	// Unchanged bytes, and no migration copy: a store already in the current
	// format is read, not converted. Rewriting it on every start would make the
	// file's mtime meaningless and bury the one migration that matters.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store back: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Error("loading a current store rewrote it")
	}
	if _, err := os.Stat(path + ".v1"); !os.IsNotExist(err) {
		t.Error("loading a current store produced a v1 migration copy")
	}
}

// TestMalformedV2IsReportedNotConverted is a guard, not a regression test.
//
// It pins the behaviour a damaged current store must get: a reported error, the
// file moved aside, no v1 migration copy, and a store that still works. What it
// does not do is prove decodeStore's format probe is what produces that — this
// test passes against the naive "Version decoded as zero means v1" version too,
// because decodeV1 unmarshals into a map of endpoint histories and a v2 document
// fails there as well, landing in the same branch. Verified by running it
// against that implementation, not assumed. The probe is still the right call —
// it names the format the bytes are in rather than inferring it from a parsed
// zero — but this file does not get to claim it as a defended bug.
func TestMalformedV2IsReportedNotConverted(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "version is not a number",
			body: `{"version": "two", "active_project": "default", "projects": [], "histories": {}}`,
		},
		{
			name: "histories is the wrong type",
			body: `{"version": 2, "active_project": "default", "projects": [], "histories": "nope"}`,
		},
		{
			name: "projects is the wrong type",
			body: `{"version": 2, "active_project": "default", "projects": 7, "histories": {}}`,
		},
		{
			name: "a version this build does not write",
			body: `{"version": 99, "active_project": "default", "projects": [], "histories": {}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := driftwoodHome(t)
			path := filepath.Join(dir, "baselines.json")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}

			store, err := NewStore("http://localhost:3000", "8787")
			if err == nil {
				t.Fatal("a damaged store document was accepted without a word")
			}
			if store == nil {
				t.Fatal("NewStore returned no store, so the app cannot start at all")
			}

			if _, err := os.Stat(path + ".v1"); !os.IsNotExist(err) {
				t.Error("a damaged current store was converted as though it were v1")
			}
			backups, _ := filepath.Glob(path + ".corrupt.*")
			if len(backups) == 0 {
				t.Error("a damaged store was not moved aside")
			}

			// The store still works, which is the point of reporting rather than
			// refusing: a file problem is recoverable, an outage is not.
			if _, err := store.SaveBaseline(store.ActiveProject(), "GET", "/api/after", `{"ok": true}`); err != nil {
				t.Errorf("the store could not be used after a damaged file: %v", err)
			}
		})
	}
}

// TestCorruptionThatCannotBeMovedAsideIsReported covers the half of the
// recovery path where the recovery itself fails.
//
// Renaming the bad file out of the way is what makes the next save safe. When
// the rename fails, the file is still sitting where the next write will land on
// it, and that is a different situation from one safely aside — so it is a
// different message. Reporting neither, or reporting the successful wording, is
// how an operator ends up believing a file was preserved that was not.
func TestCorruptionThatCannotBeMovedAsideIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, where a read-only directory is not enforced")
	}

	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")
	if err := os.WriteFile(path, []byte(`{ not json`), 0600); err != nil {
		t.Fatal(err)
	}
	// Readable, not writable: the read succeeds, the rename cannot.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	store, err := NewStore("http://localhost:3000", "8787")
	if err == nil {
		t.Fatal("an unreadable store that could not be moved aside was accepted silently")
	}
	if store == nil {
		t.Fatal("NewStore returned no store")
	}
	if !strings.Contains(err.Error(), "could not be moved aside") {
		t.Errorf("the report does not say the file is still in place: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the damaged file should still be where it was: %v", statErr)
	}
}

// TestMigrationFailureLeavesTheV1FileIntact covers the other ordering hazard.
//
// The backup is written before the converted store, never after. Written the
// other way round, the conversion has already replaced baselines.json by the
// time anything is moved aside — so the file being preserved is the new one, and
// the only copy of the old contracts is gone.
func TestMigrationFailureLeavesTheV1FileIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, where a read-only directory is not enforced")
	}

	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	now := time.Now().Truncate(time.Second)
	original := writeV1(t, path, map[string]*types.EndpointHistory{
		"GET:/api/users": v1History("GET", "/api/users", `{"id": 1}`, now),
	})

	// The backup cannot be written, so the conversion must not happen at all.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	_, loadErr := NewStore("http://localhost:3000", "8787")
	if loadErr == nil {
		t.Fatal("a conversion that could not keep a backup was reported as successful")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the store disappeared: %v", err)
	}
	if !bytes.Equal(after, original) {
		t.Error("a failed migration changed the file it was supposed to leave alone")
	}
	if !strings.Contains(loadErr.Error(), "left as it was") {
		t.Errorf("the report does not say the file was left untouched: %v", loadErr)
	}
}

// TestMigratedStoreLoadsAsCurrentOnTheNextStart is the round trip: whatever the
// conversion writes, the next start must read back as an ordinary current store
// and migrate nothing further.
func TestMigratedStoreLoadsAsCurrentOnTheNextStart(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	now := time.Now().Truncate(time.Second)
	writeV1(t, path, map[string]*types.EndpointHistory{
		"GET:/api/users": v1History("GET", "/api/users", `{"id": 1}`, now),
	})

	if _, err := NewStore("http://localhost:3000", "8787"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	converted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("second start reported an error against a store this build wrote: %v", err)
	}
	if _, ok := second.GetBaseline(second.ActiveProject(), "GET", "/api/users"); !ok {
		t.Error("the endpoint did not survive the second start")
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(converted, again) {
		t.Error("the second start rewrote a store that was already current")
	}
}
