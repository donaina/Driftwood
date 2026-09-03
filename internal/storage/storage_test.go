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
