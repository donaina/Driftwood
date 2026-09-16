package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

func TestStore_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{
			TargetURL:        "http://localhost:3000",
			ProxyPort:        "8787",
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
		alertOrder:  make([]string, 0),
	}

	cb, err := s.SaveBaseline("GET", "/api/users", `{"id": 1, "name": "Alice"}`)
	if err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}
	if cb.Version != 1 {
		t.Errorf("version = %d, want 1", cb.Version)
	}

	s2 := &Store{
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{
			TargetURL:        "http://localhost:3000",
			ProxyPort:        "8787",
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}
	if err := s2.loadHistoriesFromFile(); err != nil {
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	err := s.loadHistoriesFromFile()
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
		alertOrder:  make([]string, 0),
	}

	for i := 0; i < 10; i++ {
		_, _ = s.SaveBaseline("GET", "/api/test", `{"v": `+string(rune(i+'0'))+`}`)
	}

	data, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatal(err)
	}
	var histories map[string]*types.EndpointHistory
	if err := json.Unmarshal(data, &histories); err != nil {
		t.Errorf("atomic write produced invalid JSON: %v", err)
	}
}

func TestStore_RingBufferTraffic(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 3,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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

	alerts := s.GetAlerts(300)
	if len(alerts) > 200 {
		t.Errorf("alerts len = %d, want <= 200 (ring buffer limit)", len(alerts))
	}
}

func TestStore_GetBaseline_ReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
	}
	_ = s2.loadHistoriesFromFile()

	hist2, _ := s2.GetHistory("GET", "/api/users")
	if len(hist2.Versions) != 3 {
		t.Errorf("persisted history versions = %d, want 3", len(hist2.Versions))
	}
}

func TestStore_SeedIfAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "baselines.json")

	s := &Store{
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
		traffics:   make([]types.CapturedTraffic, 0),
		histories:  make(map[string]*types.EndpointHistory),
		alerts:     make(map[string]*types.Alert),
		maxTraffics: 500,
		persistPath: persistPath,
		config: types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
	return &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(t.TempDir(), "baselines.json"),
		config:      types.ProxyConfig{TargetURL: "http://localhost:3000"},
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
	raw, err := os.ReadFile(s.persistPath)
	if err != nil {
		t.Fatalf("reading persist file: %v", err)
	}
	var onDisk map[string]*types.EndpointHistory
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshalling persist file: %v", err)
	}
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
	if err := reloaded.loadHistoriesFromFile(); err != nil {
		t.Fatalf("loadHistoriesFromFile: %v", err)
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
