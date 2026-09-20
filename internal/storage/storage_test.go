package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

func TestStore_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{
			TargetURL:        "http://localhost:3000",
			ProxyPort:        "8787",
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
		alertOrder: make([]string, 0),
	}

	cb, err := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice"}`)
	if err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}
	if cb.Version != 1 {
		t.Errorf("version = %d, want 1", cb.Version)
	}

	s2 := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{
			TargetURL:        "http://localhost:3000",
			ProxyPort:        "8787",
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}
	if err := s2.loadFromFile(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	loaded, exists := s2.GetBaseline("GET", "/api/users")
	if !exists {
		t.Fatal("baseline not loaded")
	}
	if loaded.SamplePayload != `{"id": 1, "name": "Alice"}` {
		t.Errorf("payload = %s, want original", loaded.SamplePayload)
	}
	if loaded.Version != 1 {
		t.Errorf("loaded version = %d, want 1", loaded.Version)
	}
}

func TestStore_CorruptFileRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	corrupt := `{ "not valid json`
	if err := os.WriteFile(persistPath, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	err := s.loadFromFile()
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}

	backups, _ := filepath.Glob(persistPath + ".corrupt.*")
	if len(backups) == 0 {
		t.Error("expected corrupt backup file")
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	for i := 0; i < 10; i++ {
		_, _ = s.SaveBaseline("GET", "/api/test", `{"v": `+string(rune(i+'0'))+`}`)
	}

	if histories := readPersistedHistories(t, persistPath); histories["GET:/api/test"] == nil {
		t.Error("atomic write left no readable endpoint in the store document")
	}
}

// readPersistedHistories returns the endpoints the persisted store holds for its
// active project.
//
// Tests read the file through here rather than unmarshalling it themselves, so
// the on-disk shape is known in one place. It has changed once — the document
// gained a version key, a project list and a level of nesting around the
// histories — and every assertion that reached into the raw bytes had to change
// with it.
func readPersistedHistories(t *testing.T, path string) map[string]*types.EndpointHistory {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc persistedState
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not a readable store document: %v", path, err)
	}
	h, ok := doc.Histories[doc.Active]
	if !ok {
		t.Fatalf("%s holds no histories for its active project %q", path, doc.Active)
	}
	return h
}

func TestStore_RingBufferTraffic(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 3,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	for i := 0; i < 5; i++ {
		s.AddTraffic(types.CapturedTraffic{
			ID:         fmtID(i),
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			DurationMs: 10,
			IsJSON:     true,
		})
	}

	traffics := s.GetTraffics(10)
	if len(traffics) != 3 {
		t.Errorf("traffics len = %d, want 3 (maxTraffics)", len(traffics))
	}
}

func TestStore_RingBufferAlerts(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		// Set explicitly. It was left at its zero value, which made this test
		// assert nothing: with maxAlerts 0 every alert evicted itself, so
		// GetAlerts returned an empty slice and "len <= 200" held trivially. A
		// bound that is never approached cannot be tested by asserting it is not
		// exceeded.
		maxAlerts:   200,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	for i := 0; i < 250; i++ {
		s.AddTraffic(types.CapturedTraffic{
			ID:         fmtID(i),
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			DurationMs: 10,
			IsJSON:     true,
			Diff: &types.ContractDiff{
				Deltas: []types.DiffDelta{
					{Severity: types.SeverityBreaking, Kind: types.KindTypeMismatch},
				},
				HasBreakingChanges: true,
			},
		})
	}

	// Exactly the cap, not merely "at most": 250 alerts went in, so anything less
	// than 200 means the ring is dropping more than it should.
	if alerts := s.GetAlerts(300); len(alerts) != 200 {
		t.Errorf("alerts len = %d, want exactly 200 (the ring buffer limit)", len(alerts))
	}
}

func TestStore_GetBaseline_ReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice"}`)

	b1, _ := s.GetBaseline("GET", "/api/users")
	b2, _ := s.GetBaseline("GET", "/api/users")

	b1.SamplePayload = `{"id": 999}`

	if b2.SamplePayload == `{"id": 999}` {
		t.Error("GetBaseline returned same pointer (alias), not copy")
	}

	b3, _ := s.GetBaseline("GET", "/api/users")
	if b3.SamplePayload == `{"id": 999}` {
		t.Error("store's internal baseline was mutated via returned pointer")
	}
}

func TestStore_GetAllBaselines_ReturnsCopies(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 1}`)
	_, _ = s.SaveBaseline("POST", "/api/users", `{"id": 2}`)

	list1 := s.GetAllBaselines()
	list2 := s.GetAllBaselines()

	list1[0].SamplePayload = `{"mutated": true}`

	if list2[0].SamplePayload == `{"mutated": true}` {
		t.Error("GetAllBaselines returned same pointers (alias), not copies")
	}
}

func TestStore_VersionedHistory(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	v1, _ := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "v1"}`)
	v2, _ := s.SaveBaseline("GET", "/api/users", `{"id": 2, "name": "v2"}`)
	v3, _ := s.SaveBaseline("GET", "/api/users", `{"id": 3, "name": "v3"}`)

	if v1.Version != 1 || v2.Version != 2 || v3.Version != 3 {
		t.Errorf("versions: %d, %d, %d", v1.Version, v2.Version, v3.Version)
	}

	list := s.GetAllBaselines()
	if len(list) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(list))
	}
	if list[0].Version != 3 {
		t.Errorf("latest version = %d, want 3", list[0].Version)
	}
}

// NEW TEST: Verify version history is preserved (not just latest)
func TestStore_VersionHistoryPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	// Save 3 versions
	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "v1"}`)
	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 2, "name": "v2"}`)
	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 3, "name": "v3"}`)

	// Should be able to retrieve historical versions
	hist, exists := s.GetHistory("GET", "/api/users")
	if !exists {
		t.Fatal("history not found")
	}
	if len(hist.Versions) != 3 {
		t.Errorf("history versions = %d, want 3", len(hist.Versions))
	}
	// Verify versions are in order
	for i, v := range hist.Versions {
		if v.Version != i+1 {
			t.Errorf("version[%d] = %d, want %d", i, v.Version, i+1)
		}
	}

	// Reload from file and verify history preserved
	s2 := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
	}
	_ = s2.loadFromFile()

	hist2, _ := s2.GetHistory("GET", "/api/users")
	if len(hist2.Versions) != 3 {
		t.Errorf("persisted history versions = %d, want 3", len(hist2.Versions))
	}
}

func TestStore_SeedIfAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 1, "locked": true}`)
	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 999, "from": "seed"}`)

	loaded, _ := s.GetBaseline("GET", "/api/users")
	if loaded.Version != 2 {
		t.Errorf("version = %d, want 2 (seed-if-absent should not overwrite)", loaded.Version)
	}
}

func TestStore_FrequencyBasedRequiredKeys(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	_, _ = s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice", "email": "alice@example.com"}`)

	loaded, _ := s.GetBaseline("GET", "/api/users")
	if loaded.Schema == nil {
		t.Fatal("schema is nil")
	}
	found := false
	for _, k := range loaded.Schema.RequiredKeys {
		if k == "email" {
			found = true
		}
	}
	if !found {
		t.Error("RequiredKeys missing 'email' from first sample")
	}
}

func fmtID(i int) string {
	return "tr_" + string(rune(i+'0'))
}

// newObsStore builds a Store the way the tests above do, with the fields
// AddTraffic touches initialised. maxAlerts is set so a test that happens to
// record a diff does not silently discard the alert it just stored.
func newObsStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(dir, "baselines.json"),
		configPath:  filepath.Join(dir, "config.json"),
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000", ProxyPort: "8787"},
	}
}

func observation(method, path string, status int, duration int64) types.CapturedTraffic {
	return types.CapturedTraffic{
		// These carry no Diff, so no alert is raised and the ID is unused.
		ID:             "tr_" + method + "_" + path,
		Method:         method,
		Path:           path,
		StatusCode:     status,
		DurationMs:     duration,
		ContractStatus: "MATCH",
	}
}

func TestStore_AddTraffic_RecordsObservation(t *testing.T) {
	s := newObsStore(t)

	s.AddTraffic(observation("GET", "/api/users", 200, 12))

	h, ok := s.GetHistory("GET", "/api/users")
	if !ok {
		t.Fatal("AddTraffic did not create a history entry")
	}
	if len(h.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(h.Observations))
	}
	if got := h.Observations[0]; got.StatusCode != 200 || got.DurationMs != 12 || got.ContractStatus != "MATCH" {
		t.Errorf("observation = %+v, want status 200, duration 12, contract MATCH", got)
	}
	if h.ObservationCount != 1 {
		t.Errorf("observation_count = %d, want 1", h.ObservationCount)
	}
}

func TestStore_AddTraffic_DoesNotCreateABaseline(t *testing.T) {
	s := newObsStore(t)

	s.AddTraffic(observation("GET", "/api/users", 200, 5))

	// An endpoint that has been seen but never accepted is still unbaselined.
	// This is what makes "Driftwood found traffic it has no contract for" a
	// state the product can show rather than one it papers over.
	if _, ok := s.GetBaseline("GET", "/api/users"); ok {
		t.Error("GetBaseline returned a baseline for an endpoint that only had traffic")
	}
	h, _ := s.GetHistory("GET", "/api/users")
	if len(h.Versions) != 0 {
		t.Errorf("versions = %d, want 0", len(h.Versions))
	}
	if got := len(s.GetAllBaselines()); got != 0 {
		t.Errorf("GetAllBaselines = %d, want 0", got)
	}
}

func TestStore_ObservationWindowIsCapped(t *testing.T) {
	s := newObsStore(t)

	const total = maxObservations + 10
	for i := 0; i < total; i++ {
		s.AddTraffic(observation("GET", "/api/users", 200, int64(i)))
	}

	h, _ := s.GetHistory("GET", "/api/users")
	if len(h.Observations) != maxObservations {
		t.Fatalf("window = %d, want %d", len(h.Observations), maxObservations)
	}
	// The count is the true total even though the window forgot the rest...
	if h.ObservationCount != total {
		t.Errorf("observation_count = %d, want %d", h.ObservationCount, total)
	}
	// ...and the window kept the newest, not the oldest.
	if first, last := h.Observations[0].DurationMs, h.Observations[maxObservations-1].DurationMs; first != 10 || last != int64(total-1) {
		t.Errorf("window spans durations %d..%d, want 10..%d", first, last, total-1)
	}
}

func TestStore_SaveBaseline_DoesNotCountAsAnObservation(t *testing.T) {
	s := newObsStore(t)

	for i := 0; i < 3; i++ {
		if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice"}`); err != nil {
			t.Fatalf("SaveBaseline failed: %v", err)
		}
	}

	h, _ := s.GetHistory("GET", "/api/users")
	if h.ObservationCount != 0 {
		t.Errorf("observation_count = %d after 3 baseline saves, want 0", h.ObservationCount)
	}

	// And a real observation is not displaced by a later save.
	s.AddTraffic(observation("GET", "/api/users", 200, 5))
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 2, "name": "Bob"}`); err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}
	h, _ = s.GetHistory("GET", "/api/users")
	if h.ObservationCount != 1 || len(h.Observations) != 1 {
		t.Errorf("count = %d, observations = %d after save, want 1 and 1",
			h.ObservationCount, len(h.Observations))
	}
	// Three saves in the loop above, plus the one just now.
	if len(h.Versions) != 4 {
		t.Errorf("versions = %d, want 4", len(h.Versions))
	}
}

func TestStore_ObservationsAreNotPersisted(t *testing.T) {
	s := newObsStore(t)
	s.AddTraffic(observation("GET", "/api/users", 200, 7))
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1}`); err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	// The file holds contracts. Traffic telemetry is per-process, the same way
	// the traffic ring buffer is, so a restart starts from zero observations
	// rather than from a snapshot of some previous run.
	onDisk := readPersistedHistories(t, s.persistPath)
	entry := onDisk["GET:/api/users"]
	if entry == nil {
		t.Fatal("endpoint missing from persist file")
	}
	if len(entry.Versions) != 1 {
		t.Errorf("persisted versions = %d, want 1", len(entry.Versions))
	}
	if len(entry.Observations) != 0 || entry.ObservationCount != 0 {
		t.Errorf("persisted %d observations / count %d, want 0 and 0",
			len(entry.Observations), entry.ObservationCount)
	}

	// Same through a real reload.
	reloaded := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxAlerts:   200,
		persistPath: s.persistPath,
	}
	if err := reloaded.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	h, ok := reloaded.GetHistory("GET", "/api/users")
	if !ok {
		t.Fatal("endpoint did not survive reload")
	}
	if len(h.Observations) != 0 || h.ObservationCount != 0 {
		t.Errorf("after reload: %d observations / count %d, want 0 and 0",
			len(h.Observations), h.ObservationCount)
	}
	if len(h.Versions) != 1 {
		t.Errorf("after reload: versions = %d, want 1", len(h.Versions))
	}
}

func TestStore_GetHistory_ObservationsAreACopy(t *testing.T) {
	s := newObsStore(t)
	s.AddTraffic(observation("GET", "/api/users", 200, 1))

	before, _ := s.GetHistory("GET", "/api/users")
	s.AddTraffic(observation("GET", "/api/users", 500, 2))

	if len(before.Observations) != 1 {
		t.Errorf("a previously returned history grew to %d observations", len(before.Observations))
	}

	// Appending to what the store handed back must not reach into the store.
	after, _ := s.GetHistory("GET", "/api/users")
	after.Observations[0].StatusCode = 0
	after.Observations = append(after.Observations, types.Observation{StatusCode: 999})

	again, _ := s.GetHistory("GET", "/api/users")
	if len(again.Observations) != 2 {
		t.Errorf("store now holds %d observations, want 2", len(again.Observations))
	}
	if again.Observations[0].StatusCode != 200 {
		t.Errorf("store observation was mutated through the returned copy: status = %d",
			again.Observations[0].StatusCode)
	}
}

func TestStore_GetAllHistories_ObservationsAreACopy(t *testing.T) {
	s := newObsStore(t)
	s.AddTraffic(observation("GET", "/api/users", 200, 1))

	list := s.GetAllHistories()
	if len(list) != 1 {
		t.Fatalf("histories = %d, want 1", len(list))
	}
	list[0].Observations = append(list[0].Observations, types.Observation{StatusCode: 999})

	again := s.GetAllHistories()
	if len(again[0].Observations) != 1 {
		t.Errorf("store now holds %d observations, want 1", len(again[0].Observations))
	}
}

func TestStore_DeleteBaseline_KeepsObservations(t *testing.T) {
	s := newObsStore(t)
	s.AddTraffic(observation("GET", "/api/users", 200, 1))
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1}`); err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	s.DeleteBaseline("GET", "/api/users")

	if _, ok := s.GetBaseline("GET", "/api/users"); ok {
		t.Error("baseline still present after DeleteBaseline")
	}
	h, ok := s.GetHistory("GET", "/api/users")
	if !ok {
		t.Fatal("DeleteBaseline discarded the endpoint's history")
	}
	if len(h.Versions) != 0 || h.LockedVersion != 0 {
		t.Errorf("versions = %d, lock = %d, want 0 and 0", len(h.Versions), h.LockedVersion)
	}
	if len(h.Observations) != 1 || h.ObservationCount != 1 {
		t.Errorf("observations = %d, count = %d, want 1 and 1",
			len(h.Observations), h.ObservationCount)
	}
}

func TestStore_ConcurrentTrafficAndReads(t *testing.T) {
	s := newObsStore(t)
	s.AddTraffic(observation("GET", "/api/users", 200, 1))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			s.AddTraffic(observation("GET", "/api/users", 200, int64(i)))
		}
	}()

	for i := 0; i < 500; i++ {
		if h, ok := s.GetHistory("GET", "/api/users"); ok {
			for range h.Observations {
			}
		}
		for _, h := range s.GetAllHistories() {
			for range h.Observations {
			}
		}
	}
	<-done
}

func TestStore_SetLockedVersion_PinsAndSurvivesReload(t *testing.T) {
	s := newObsStore(t)
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice"}`); err != nil {
		t.Fatalf("SaveBaseline v1: %v", err)
	}
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice", "role": "admin"}`); err != nil {
		t.Fatalf("SaveBaseline v2: %v", err)
	}

	// Unpinned, the endpoint is compared against its latest version.
	if b, _ := s.GetBaseline("GET", "/api/users"); b.Version != 2 {
		t.Fatalf("unpinned baseline version = %d, want 2", b.Version)
	}

	if err := s.SetLockedVersion("GET", "/api/users", 1); err != nil {
		t.Fatalf("SetLockedVersion: %v", err)
	}
	// Pinning is what makes the lock an operation rather than a badge: v2 is
	// still an accepted version, but v1 is the contract the endpoint is held to.
	b, ok := s.GetBaseline("GET", "/api/users")
	if !ok {
		t.Fatal("GetBaseline found nothing after locking")
	}
	if b.Version != 1 {
		t.Errorf("locked baseline version = %d, want 1", b.Version)
	}
	if b.SamplePayload != `{"id": 1, "name": "Alice"}` {
		t.Errorf("locked payload = %q, want the v1 payload", b.SamplePayload)
	}

	reloaded := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxAlerts:   200,
		persistPath: s.persistPath,
	}
	if err := reloaded.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if b, _ := reloaded.GetBaseline("GET", "/api/users"); b.Version != 1 {
		t.Errorf("after reload: baseline version = %d, want 1 (the pin did not persist)", b.Version)
	}

	// 0 releases the pin and the endpoint tracks its latest version again.
	if err := reloaded.SetLockedVersion("GET", "/api/users", 0); err != nil {
		t.Fatalf("releasing the lock: %v", err)
	}
	if b, _ := reloaded.GetBaseline("GET", "/api/users"); b.Version != 2 {
		t.Errorf("after release: baseline version = %d, want 2", b.Version)
	}
}

func TestStore_SetLockedVersion_RejectsUnknownVersion(t *testing.T) {
	s := newObsStore(t)
	if _, err := s.SaveBaseline("GET", "/api/users", `{"id": 1}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	if err := s.SetLockedVersion("GET", "/api/users", 7); err == nil {
		t.Error("locking to a version that does not exist should fail")
	}
	if err := s.SetLockedVersion("GET", "/nowhere", 1); err == nil {
		t.Error("locking an endpoint with no history should fail")
	}
	if b, _ := s.GetBaseline("GET", "/api/users"); b.Version != 1 {
		t.Errorf("a rejected lock changed the baseline to version %d", b.Version)
	}
}

func TestStore_UpdateConfig_Persists(t *testing.T) {
	s := newObsStore(t)
	cfg := s.GetConfig()
	cfg.TargetURL = "http://localhost:4242"
	cfg.AutoSaveBaseline = false

	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// A fresh store, same directory, no flags: the saved settings come back.
	// This is what a restart looks like, and it is the whole point — UpdateConfig
	// used to mutate memory only, so every setting reverted on restart.
	restarted := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxAlerts:   200,
		persistPath: s.persistPath,
		configPath:  s.configPath,
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000", ProxyPort: "8787", AutoSaveBaseline: true},
	}
	if err := restarted.ApplyRememberedConfig(false, false); err != nil {
		t.Fatalf("ApplyRememberedConfig: %v", err)
	}
	got := restarted.GetConfig()
	if got.TargetURL != "http://localhost:4242" {
		t.Errorf("target = %q, want the saved http://localhost:4242", got.TargetURL)
	}
	if got.AutoSaveBaseline {
		t.Error("auto_save_baseline = true, want the saved false")
	}
}

func TestStore_ApplyRememberedConfig_ExplicitFlagsWin(t *testing.T) {
	s := newObsStore(t)
	cfg := s.GetConfig()
	cfg.TargetURL = "http://saved.example:9000"
	cfg.ProxyPort = "9999"
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// A flag the user actually typed is a decision about this run and outranks
	// the remembered preference; one left at its default is not a decision, so
	// the saved value stands. Which is which is passed in, because comparing
	// against the default would read "explicitly set to the default" as silence.
	flagged := &Store{
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		persistPath: s.persistPath,
		configPath:  s.configPath,
		config:      types.ProxyConfig{TargetURL: "http://typed.example:1234", ProxyPort: "8787"},
	}
	if err := flagged.ApplyRememberedConfig(true, false); err != nil {
		t.Fatalf("ApplyRememberedConfig: %v", err)
	}
	got := flagged.GetConfig()
	if got.TargetURL != "http://typed.example:1234" {
		t.Errorf("target = %q, want the explicit flag value", got.TargetURL)
	}
	if got.ProxyPort != "9999" {
		t.Errorf("port = %q, want the saved 9999", got.ProxyPort)
	}
}

func TestStore_ApplyRememberedConfig_MissingAndCorruptFiles(t *testing.T) {
	s := newObsStore(t)

	// Missing is the normal first run, not an error.
	if err := s.ApplyRememberedConfig(false, false); err != nil {
		t.Errorf("missing config file returned an error: %v", err)
	}
	if got := s.GetConfig().TargetURL; got != "http://localhost:3000" {
		t.Errorf("target = %q, want the default to survive", got)
	}

	if err := os.WriteFile(s.configPath, []byte("{not json"), 0600); err != nil {
		t.Fatalf("writing a corrupt config: %v", err)
	}
	if err := s.ApplyRememberedConfig(false, false); err == nil {
		t.Error("a corrupt config file should be reported")
	}
	// Reported, moved aside, and startup continues on the defaults rather than
	// being blocked by a file the user cannot see or edit.
	if got := s.GetConfig().TargetURL; got != "http://localhost:3000" {
		t.Errorf("target after a corrupt file = %q, want the default", got)
	}
	matches, _ := filepath.Glob(s.configPath + ".corrupt.*")
	if len(matches) != 1 {
		t.Errorf("corrupt config files matching %q = %d, want 1", s.configPath+".corrupt.*", len(matches))
	}
}

/*
Configured, which is what the dashboard's first-run wizard keys off.

	The bug this replaces: the wizard asked whether the store held zero
	baselines. Driftwood seeds a contract for its own mock simulator, so a fresh
	install never had zero, and auto-baselining means any stray request — a
	browser's GET /favicon.ico is the one that actually happened — also made an
	untouched install look set up. Both are traffic, not intent. This flag is
	intent, so each way of expressing intent is asserted separately.
*/
func TestStore_IsConfigured(t *testing.T) {
	t.Run("a fresh store is not configured", func(t *testing.T) {
		s := newObsStore(t)
		if s.IsConfigured() {
			t.Error("a store nobody has touched reported itself as configured")
		}
	})

	t.Run("a seeded baseline does not configure it", func(t *testing.T) {
		s := newObsStore(t)
		if _, err := s.SaveBaseline("GET", "/_driftwood/mock/users", `{"id":1}`); err != nil {
			t.Fatalf("seeding a baseline: %v", err)
		}
		if s.IsConfigured() {
			t.Error("captured traffic was mistaken for the operator naming a target")
		}
	})

	t.Run("saving a config configures it", func(t *testing.T) {
		s := newObsStore(t)
		if err := s.UpdateConfig(types.ProxyConfig{TargetURL: "http://localhost:9000"}); err != nil {
			t.Fatalf("UpdateConfig: %v", err)
		}
		if !s.IsConfigured() {
			t.Error("a saved config did not mark the install configured")
		}
	})

	t.Run("a config on disk configures it after a restart", func(t *testing.T) {
		s := newObsStore(t)
		if err := s.UpdateConfig(types.ProxyConfig{TargetURL: "http://localhost:9000"}); err != nil {
			t.Fatalf("UpdateConfig: %v", err)
		}

		restarted := newObsStore(t)
		restarted.configPath = s.configPath
		if err := restarted.ApplyRememberedConfig(false, false); err != nil {
			t.Fatalf("ApplyRememberedConfig: %v", err)
		}
		if !restarted.IsConfigured() {
			t.Error("a remembered config did not survive a restart")
		}
	})
}

/* Provenance, and the auto-save trap it closes.

   A version captured from live traffic is a guess: Driftwood saw one response
   and inferred a contract from it. If the API was already drifted when that
   happened, the drift is inside the baseline, so every later response matches
   it and the endpoint reads healthy forever. Nothing in the process can detect
   that — only a person can, and only if the version is marked as unconfirmed.
   These tests pin the marking, not a detection we cannot do. */

func TestStore_SaveBaseline_RecordsItsSource(t *testing.T) {
	s := newObsStore(t)

	manual, err := s.SaveBaseline("GET", "/api/users", `{"id": 1}`)
	if err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	if manual.Source != types.BaselineSourceManual {
		t.Errorf("SaveBaseline source = %q, want %q", manual.Source, types.BaselineSourceManual)
	}
	if manual.IsProvisional() {
		t.Error("a version a human saved reads as provisional")
	}

	auto, err := s.SaveBaselineFrom("GET", "/api/orders", `{"id": 2}`, types.BaselineSourceAuto)
	if err != nil {
		t.Fatalf("SaveBaselineFrom: %v", err)
	}
	if !auto.IsProvisional() {
		t.Error("a version captured from live traffic does not read as provisional")
	}
}

func TestStore_LegacyBaselineWithoutSourceReadsAsProvisional(t *testing.T) {
	// Baselines written before Source existed carry no value. They were, in
	// nearly every case, auto-captured — the promote route had no UI — so
	// reading them as confirmed would assert a human vouched for each one.
	legacy := &types.ContractBaseline{Version: 1}
	if !legacy.IsProvisional() {
		t.Error("a baseline with no source reads as confirmed")
	}
	spec := &types.ContractBaseline{Version: 1, Source: types.BaselineSourceSpec}
	if spec.IsProvisional() {
		t.Error("an imported spec reads as provisional")
	}
}

func TestStore_ConfirmBaseline_UnmarksProvisional(t *testing.T) {
	s := newObsStore(t)
	if _, err := s.SaveBaselineFrom("GET", "/api/users", `{"id": 1}`, types.BaselineSourceAuto); err != nil {
		t.Fatalf("SaveBaselineFrom: %v", err)
	}

	if err := s.ConfirmBaseline("GET", "/api/users", 1); err != nil {
		t.Fatalf("ConfirmBaseline: %v", err)
	}

	hist, ok := s.GetHistory("GET", "/api/users")
	if !ok {
		t.Fatal("history for GET /api/users not found")
	}
	if hist.Versions[0].IsProvisional() {
		t.Error("version still provisional after confirm")
	}
	if hist.Versions[0].Source != types.BaselineSourceManual {
		t.Errorf("source after confirm = %q, want %q", hist.Versions[0].Source, types.BaselineSourceManual)
	}
}

func TestStore_ConfirmBaseline_SurvivesReload(t *testing.T) {
	s := newObsStore(t)
	if _, err := s.SaveBaselineFrom("GET", "/api/users", `{"id": 1}`, types.BaselineSourceAuto); err != nil {
		t.Fatalf("SaveBaselineFrom: %v", err)
	}
	if err := s.ConfirmBaseline("GET", "/api/users", 1); err != nil {
		t.Fatalf("ConfirmBaseline: %v", err)
	}

	reloaded := newObsStore(t)
	reloaded.persistPath = s.persistPath
	reloaded.configPath = s.configPath
	if err := reloaded.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}

	hist, ok := reloaded.GetHistory("GET", "/api/users")
	if !ok {
		t.Fatal("history did not survive a reload")
	}
	if hist.Versions[0].IsProvisional() {
		t.Error("the confirmation was not persisted, so a restart un-confirms the contract")
	}
}

func TestStore_ConfirmBaseline_RejectsBadInput(t *testing.T) {
	s := newObsStore(t)
	if _, err := s.SaveBaselineFrom("GET", "/api/users", `{"id": 1}`, types.BaselineSourceAuto); err != nil {
		t.Fatalf("SaveBaselineFrom: %v", err)
	}

	tests := []struct {
		name    string
		method  string
		path    string
		version int
	}{
		{"unknown endpoint", "GET", "/api/nope", 1},
		{"version past the end", "GET", "/api/users", 2},
		{"version zero is not 'the latest'", "GET", "/api/users", 0},
		{"negative version", "GET", "/api/users", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.ConfirmBaseline(tc.method, tc.path, tc.version); err == nil {
				t.Errorf("ConfirmBaseline(%s %s, %d) = nil, want an error", tc.method, tc.path, tc.version)
			}
		})
	}
}

func TestStore_ConfirmBaseline_IsIdempotentOnAConfirmedVersion(t *testing.T) {
	// Confirming twice asks for a state the caller already has. Reporting an
	// error would turn a successful end state into a failure message.
	s := newObsStore(t)
	if _, err := s.SaveBaselineFrom("GET", "/api/users", `{"id": 1}`, types.BaselineSourceAuto); err != nil {
		t.Fatalf("SaveBaselineFrom: %v", err)
	}
	if err := s.ConfirmBaseline("GET", "/api/users", 1); err != nil {
		t.Fatalf("first ConfirmBaseline: %v", err)
	}
	if err := s.ConfirmBaseline("GET", "/api/users", 1); err != nil {
		t.Errorf("second ConfirmBaseline = %v, want nil", err)
	}
}

// TestStore_EndpointCapEvictsOnlyWhatItMayForget covers the bound on the history
// map.
//
// One entry is created per distinct METHOD:PATH observed, and a path carrying a
// parameter creates one per entity — /orders/1, /orders/2 — so the map grew once
// per request, each entry able to hold a baseline body. The cap evicts the
// endpoint that has gone longest without being seen, and this test is mostly
// about the endpoints it must not touch: dropping a contract somebody accepted
// does not merely lose a record, it leaves the endpoint unbaselined and hands
// the next response the job of defining the contract.
func TestStore_EndpointCapEvictsOnlyWhatItMayForget(t *testing.T) {
	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(t.TempDir(), "baselines.json"),
		config: types.ProxyConfig{
			TargetURL:        "http://localhost:3000",
			ProxyPort:        "8787",
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}

	observe := func(path string) {
		t.Helper()
		s.AddTraffic(types.CapturedTraffic{
			ID:         path,
			Method:     "GET",
			Path:       path,
			Timestamp:  time.Now(),
			StatusCode: 200,
		})
	}

	for i := 0; i < maxEndpoints; i++ {
		observe(fmt.Sprintf("/filler/%d", i))
	}
	if got := len(s.GetAllHistories()); got != maxEndpoints {
		t.Fatalf("histories = %d after %d distinct endpoints, want %d", got, maxEndpoints, maxEndpoints)
	}

	// /filler/0 is pinned to a version and /filler/1 carries a version a human
	// accepted. Neither may be spent to make room for a stranger.
	if _, err := s.SaveBaselineFrom("GET", "/filler/0", `{"id": 1}`, types.BaselineSourceManual); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLockedVersion("GET", "/filler/0", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveBaselineFrom("GET", "/filler/1", `{"id": 1}`, types.BaselineSourceManual); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < maxEndpoints; i++ {
		observe(fmt.Sprintf("/new/%d", i))
	}

	if _, ok := s.GetHistory("GET", "/filler/0"); !ok {
		t.Error("an endpoint pinned to a version was evicted to make room")
	}
	if _, ok := s.GetHistory("GET", "/filler/1"); !ok {
		t.Error("an endpoint with a confirmed version was evicted to make room")
	}
	// The newest arrival survives because it is the one that did the evicting.
	// Once the filler is exhausted the series cycles: with the two protected
	// endpoints fixed in place, each new path spends the least recently seen
	// provisional entry, so the map holds a window of recent endpoints rather
	// than the first ones it ever saw.
	if _, ok := s.GetHistory("GET", fmt.Sprintf("/new/%d", maxEndpoints-1)); !ok {
		t.Error("the endpoint seen most recently was not recorded")
	}
	if _, ok := s.GetHistory("GET", "/filler/2"); ok {
		t.Error("an endpoint nothing protects was never evicted: the map is not bounded after all")
	}
	if got := len(s.GetAllHistories()); got > maxEndpoints {
		t.Errorf("histories = %d, want the map held at its cap of %d", got, maxEndpoints)
	}
}

// TestPersistingMethodsSerialiseOnTheWriteLock is the regression test for the
// unserialised persist: it asserts the mechanism, because the symptom cannot be
// forced.
//
// The bug was that each save took its snapshot under mu and then wrote after
// releasing it, so two saves could snapshot in one order and write in the other
// and the file kept whichever landed last, losing a whole endpoint with no parse
// error and no log line. Reproducing that needs a goroutine descheduled in the
// narrow window between the snapshot and the write, which no amount of
// concurrency in a test can guarantee — see the note on
// TestConcurrentSavesLandComplete, which checks the end state but cannot make
// the losing interleaving happen.
//
// What can be tested deterministically is the fix itself: holding writeMx must
// block every method that persists. On the old code each of these returned
// immediately. Note this covers the lock, not the ordering — an implementation
// that took writeMx and then dropped it before writing would pass here and still
// lose writes.
func TestPersistingMethodsSerialiseOnTheWriteLock(t *testing.T) {
	cases := []struct {
		name string
		call func(*Store) error
	}{
		{"SaveBaseline", func(s *Store) error {
			_, err := s.SaveBaseline("GET", "/api/x", `{"a": 1}`)
			return err
		}},
		{"SetLockedVersion", func(s *Store) error {
			return s.SetLockedVersion("GET", "/api/x", 1)
		}},
		{"ConfirmBaseline", func(s *Store) error {
			return s.ConfirmBaseline("GET", "/api/x", 1)
		}},
		{"DeleteBaseline", func(s *Store) error {
			s.DeleteBaseline("GET", "/api/x")
			return nil
		}},
		{"UpdateConfig", func(s *Store) error {
			cfg := s.GetConfig()
			return s.UpdateConfig(cfg)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newPersistingStore(t)

			// Seed the endpoint, so the calls that address a version have one.
			if _, err := s.SaveBaseline("GET", "/api/x", `{"a": 1}`); err != nil {
				t.Fatalf("seeding the endpoint: %v", err)
			}

			s.writeMx.Lock()
			done := make(chan error, 1)
			go func() { done <- tc.call(s) }()

			select {
			case err := <-done:
				s.writeMx.Unlock()
				t.Fatalf("%s completed while the write lock was held (err = %v): "+
					"it persists without serialising against another writer", tc.name, err)
			case <-time.After(200 * time.Millisecond):
				// Blocked, which is the point.
			}
			s.writeMx.Unlock()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("%s failed once the write lock was released: %v", tc.name, err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s never finished after the write lock was released", tc.name)
			}
		})
	}
}

func newPersistingStore(t *testing.T) *Store {
	t.Helper()
	return &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(t.TempDir(), "baselines.json"),
		configPath:  filepath.Join(t.TempDir(), "config.json"),
	}
}

// TestConcurrentSavesLandComplete checks the end state of a burst of concurrent
// saves: the file is what survives a restart, so it is what has to be whole.
//
// This is a guard, not a reproduction. It passes against the unserialised code
// as well, because losing a write needs the writing goroutine to be descheduled
// between its snapshot and its write, and nothing here can force that. It is
// kept because it will catch a *wider* version of the same mistake — a
// regression that drops writes routinely rather than occasionally — and because
// the assertion it makes is the one that actually matters to a user.
func TestConcurrentSavesLandComplete(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: persistPath,
	}

	const saves = 24
	var wg sync.WaitGroup
	errs := make([]error, saves)
	for i := 0; i < saves; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.SaveBaseline(
				"GET",
				fmt.Sprintf("/api/thing/%d", i),
				fmt.Sprintf(`{"n": %d}`, i),
			)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("save %d failed: %v", i, err)
		}
	}

	onDisk := readPersistedHistories(t, persistPath)

	if len(onDisk) != saves {
		var missing []string
		for i := 0; i < saves; i++ {
			if _, ok := onDisk[fmt.Sprintf("GET:/api/thing/%d", i)]; !ok {
				missing = append(missing, fmt.Sprintf("/api/thing/%d", i))
			}
		}
		t.Errorf("the persisted file holds %d endpoints, want %d; lost: %v",
			len(onDisk), saves, missing)
	}
}

// TestSaveBaselineReturnsACopy guards the other half of the same problem: the
// value handed back to the caller is in the store's map, so returning it
// directly hands out a handle on store state that no lock guards.
func TestSaveBaselineReturnsACopy(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(tmpDir, "baselines.json"),
	}

	got, err := s.SaveBaseline("GET", "/api/users", `{"id": 1}`)
	if err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	got.SamplePayload = "tampered"
	if got.Schema != nil {
		got.Schema.Type = "tampered"
	}

	stored, ok := s.GetBaseline("GET", "/api/users")
	if !ok {
		t.Fatal("baseline not in the store after saving it")
	}
	if stored.SamplePayload == "tampered" {
		t.Error("mutating the returned baseline changed the store's copy")
	}
	if stored.Schema != nil && stored.Schema.Type == "tampered" {
		t.Error("mutating the returned baseline's schema changed the store's copy")
	}
}

// TestAlertEvictionDropsTheOldest pins the ring behaviour that AddTraffic's
// eviction branch implements, which was rewritten to stop reslicing forward.
//
// The reslice was an allocation bug rather than a behavioural one, so the
// observable behaviour is what is asserted here: the cap holds, the oldest
// alerts go, and the newest survive. GetAlerts walks alertOrder backwards, so
// the most recent alert is the first element of its result.
func TestAlertEvictionDropsTheOldest(t *testing.T) {
	s := newObsStore(t)
	s.maxAlerts = 3

	alerting := func(id, path string) types.CapturedTraffic {
		return types.CapturedTraffic{
			ID:             id,
			Method:         "GET",
			Path:           path,
			StatusCode:     200,
			ContractStatus: "BREAKING",
			Diff: &types.ContractDiff{
				HasBreakingChanges: true,
			},
		}
	}

	for i := 0; i < 6; i++ {
		s.AddTraffic(alerting(fmt.Sprintf("tr_%d", i), fmt.Sprintf("/api/thing/%d", i)))
	}

	if len(s.alertOrder) != 3 {
		t.Errorf("alertOrder holds %d entries, want it capped at 3", len(s.alertOrder))
	}
	if len(s.alerts) != 3 {
		t.Errorf("alerts holds %d entries, want 3 — eviction must drop the map entry too, "+
			"or the map grows while the order slice does not", len(s.alerts))
	}

	got := s.GetAlerts(50)
	if len(got) != 3 {
		t.Fatalf("GetAlerts returned %d alerts, want 3", len(got))
	}
	// Newest first: /api/thing/5, 4, 3. The first three are evicted.
	want := []string{"/api/thing/5", "/api/thing/4", "/api/thing/3"}
	for i, path := range want {
		if !strings.Contains(got[i].Endpoint, path) {
			t.Errorf("alert %d is %q, want the endpoint containing %q",
				i, got[i].Endpoint, path)
		}
	}
	for _, a := range got {
		if strings.Contains(a.Endpoint, "/api/thing/0") ||
			strings.Contains(a.Endpoint, "/api/thing/1") ||
			strings.Contains(a.Endpoint, "/api/thing/2") {
			t.Errorf("evicted endpoint %q is still being returned", a.Endpoint)
		}
	}
}
