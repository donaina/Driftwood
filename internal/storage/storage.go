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
	writeMx     sync.Mutex // separate lock for file writes to avoid holding main lock
}

func NewStore(targetURL, proxyPort string) *Store {
	homeDir, _ := os.UserHomeDir()
	persistDir := filepath.Join(homeDir, ".driftwood")
	_ = os.MkdirAll(persistDir, 0700) // secure perms

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

	traffics := append([]types.CapturedTraffic{t}, s.traffics...)
	if len(traffics) > s.maxTraffics {
		traffics = traffics[:s.maxTraffics]
	}
	s.traffics = traffics

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

// GetBaseline returns a COPY to avoid pointer aliasing
func (s *Store) GetBaseline(method, path string) (*types.ContractBaseline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", method, path)
	b, exists := s.baselines[key]
	if !exists {
		return nil, false
	}
	// Return copy
	copy := *b
	// Deep copy schema
	copy.Schema = deepCopySchema(b.Schema)
	return &copy, true
}

// GetAllBaselines returns copies to avoid pointer aliasing
func (s *Store) GetAllBaselines() []*types.ContractBaseline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*types.ContractBaseline, 0, len(s.baselines))
	for _, b := range s.baselines {
		copy := *b
		copy.Schema = deepCopySchema(b.Schema)
		list = append(list, &copy)
	}
	return list
}

func deepCopySchema(node *types.JSONSchemaNode) *types.JSONSchemaNode {
	if node == nil {
		return nil
	}
	copy := *node
	if node.Properties != nil {
		copy.Properties = make(map[string]*types.JSONSchemaNode, len(node.Properties))
		for k, v := range node.Properties {
			copy.Properties[k] = deepCopySchema(v)
		}
	}
	if node.ItemSchema != nil {
		copy.ItemSchema = deepCopySchema(node.ItemSchema)
	}
	return &copy
}

func (s *Store) SaveBaseline(method, path, samplePayload string) (*types.ContractBaseline, error) {
	s.mu.Lock()

	inferredSchema, err := schema.InferFromJSON(samplePayload)
	if err != nil {
		s.mu.Unlock()
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

	// Marshal under lock, write outside lock
	var data []byte
	data, err = json.MarshalIndent(s.baselines, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	// Atomic write: temp file + rename
	if err := atomicWriteFile(s.persistPath, data, 0600); err != nil {
		return nil, fmt.Errorf("atomic write failed: %w", err)
	}
	return cb, nil
}

func (s *Store) DeleteBaseline(method, path string) {
	s.mu.Lock()
	key := fmt.Sprintf("%s:%s", method, path)
	delete(s.baselines, key)

	// Marshal under lock
	var data []byte
	var err error
	data, err = json.MarshalIndent(s.baselines, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return
	}
	// Atomic write outside lock
	_ = atomicWriteFile(s.persistPath, data, 0600)
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

// atomicWriteFile writes to a temp file then renames (atomic on POSIX)
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".baselines.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Store) loadBaselinesFromFile() error {
	data, err := os.ReadFile(s.persistPath)
	if err != nil {
		return err // file doesn't exist is OK
	}
	// Use a temp map to avoid partial state on corrupt file
	var baselines map[string]*types.ContractBaseline
	if err := json.Unmarshal(data, &baselines); err != nil {
		// Backup corrupt file
		_ = os.Rename(s.persistPath, s.persistPath+".corrupt."+time.Now().Format("20060102-150405"))
		return err
	}
	s.mu.Lock()
	s.baselines = baselines
	s.mu.Unlock()
	return nil
}