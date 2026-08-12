package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
)

type Store struct {
	mu          sync.RWMutex
	traffics    []types.CapturedTraffic
	baselines   map[string]*types.ContractBaseline // Key: "METHOD:PATH"
	alerts      []types.DiffDelta
	config      types.ProxyConfig
	maxTraffics int
	persistPath string
}

func NewStore(targetURL, proxyPort string) *Store {
	homeDir, _ := os.UserHomeDir()
	persistDir := filepath.Join(homeDir, ".driftwood")
	_ = os.MkdirAll(persistDir, 0755)

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		baselines:   make(map[string]*types.ContractBaseline),
		alerts:      make([]types.DiffDelta, 0),
		maxTraffics: 500,
		persistPath: filepath.Join(persistDir, "baselines.json"),
		config: types.ProxyConfig{
			TargetURL:        targetURL,
			ProxyPort:        proxyPort,
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}

	_ = s.loadBaselinesFromFile()
	return s
}

func (s *Store) GetConfig() types.ProxyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

func (s *Store) UpdateConfig(cfg types.ProxyConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

func (s *Store) AddTraffic(t types.CapturedTraffic) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.traffics = append([]types.CapturedTraffic{t}, s.traffics...)
	if len(s.traffics) > s.maxTraffics {
		s.traffics = s.traffics[:s.maxTraffics]
	}

	if t.Diff != nil && len(t.Diff.Deltas) > 0 {
		for _, d := range t.Diff.Deltas {
			if d.Severity == types.SeverityBreaking || d.Severity == types.SeverityWarning {
				s.alerts = append([]types.DiffDelta{d}, s.alerts...)
			}
		}
		if len(s.alerts) > 200 {
			s.alerts = s.alerts[:200]
		}
	}
}

func (s *Store) GetTraffics(limit int) []types.CapturedTraffic {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.traffics) {
		limit = len(s.traffics)
	}
	result := make([]types.CapturedTraffic, limit)
	copy(result, s.traffics[:limit])
	return result
}

func (s *Store) ClearTraffic() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traffics = make([]types.CapturedTraffic, 0)
}

func (s *Store) GetBaseline(method, path string) (*types.ContractBaseline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", method, path)
	b, exists := s.baselines[key]
	return b, exists
}

func (s *Store) SaveBaseline(method, path, samplePayload string) (*types.ContractBaseline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inferredSchema, err := schema.InferFromJSON(samplePayload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}

	key := fmt.Sprintf("%s:%s", method, path)
	existing, exists := s.baselines[key]

	version := 1
	var reqCount int64 = 1
	if exists {
		version = existing.Version + 1
		reqCount = existing.RequestCount + 1
	}

	cb := &types.ContractBaseline{
		ID:            fmt.Sprintf("bl_%d", time.Now().UnixNano()),
		Method:        method,
		Path:          path,
		Schema:        inferredSchema,
		SamplePayload: samplePayload,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       version,
		RequestCount:  reqCount,
	}

	if exists {
		cb.CreatedAt = existing.CreatedAt
	}

	s.baselines[key] = cb
	_ = s.saveBaselinesToFileUnsafe()
	return cb, nil
}

func (s *Store) DeleteBaseline(method, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", method, path)
	delete(s.baselines, key)
	_ = s.saveBaselinesToFileUnsafe()
}

func (s *Store) GetAllBaselines() []*types.ContractBaseline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*types.ContractBaseline, 0, len(s.baselines))
	for _, b := range s.baselines {
		list = append(list, b)
	}
	return list
}

func (s *Store) GetAlerts(limit int) []types.DiffDelta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.alerts) {
		limit = len(s.alerts)
	}
	res := make([]types.DiffDelta, limit)
	copy(res, s.alerts[:limit])
	return res
}

func (s *Store) saveBaselinesToFileUnsafe() error {
	data, err := json.MarshalIndent(s.baselines, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.persistPath, data, 0644)
}

func (s *Store) loadBaselinesFromFile() error {
	data, err := os.ReadFile(s.persistPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.baselines)
}
