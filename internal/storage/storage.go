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
	mu           sync.RWMutex
	traffics     []types.CapturedTraffic
	histories    map[string]*types.EndpointHistory // Key: "METHOD:PATH"
	alerts       map[string]*types.Alert
	alertOrder   []string
	config       types.ProxyConfig
	maxTraffics  int
	maxAlerts    int
	persistPath  string
	persistDir   string
	writeMx      sync.Mutex // separate lock for file writes
}

func NewStore(targetURL, proxyPort string) *Store {
	homeDir, _ := os.UserHomeDir()
	persistDir := filepath.Join(homeDir, ".driftwood")
	_ = os.MkdirAll(persistDir, 0700)

	s := &Store{
		traffics:    make([]types.CapturedTraffic, 0),
		histories:   make(map[string]*types.EndpointHistory),
		alerts:      make(map[string]*types.Alert),
		alertOrder:  make([]string, 0),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(persistDir, "baselines.json"),
		persistDir:  persistDir,
		config: types.ProxyConfig{
			TargetURL:        targetURL,
			ProxyPort:        proxyPort,
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}

	_ = s.loadHistoriesFromFile()
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

	if t.Diff != nil && (t.Diff.HasBreakingChanges || t.Diff.HasWarnings) {
		alert := &types.Alert{
			TrafficID:      t.ID,
			Endpoint:       fmt.Sprintf("%s %s", t.Method, t.Path),
			ContractStatus: t.ContractStatus,
			Diff:           t.Diff,
			AIExplanation:  nil,
		}
		key := t.ID
		s.alerts[key] = alert
		s.alertOrder = append(s.alertOrder, key)
		if len(s.alertOrder) > s.maxAlerts {
			oldKey := s.alertOrder[0]
			delete(s.alerts, oldKey)
			s.alertOrder = s.alertOrder[1:]
		}
	}
}

func (s *Store) UpdateAlertAIExplanation(trafficID string, explanation map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if alert, exists := s.alerts[trafficID]; exists {
		alert.AIExplanation = explanation
		return nil
	}
	return fmt.Errorf("alert not found for trafficID: %s", trafficID)
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

// GetBaseline returns a COPY of the latest (or locked) version
func (s *Store) GetBaseline(method, path string) (*types.ContractBaseline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", method, path)
	h, exists := s.histories[key]
	if !exists || len(h.Versions) == 0 {
		return nil, false
	}

	versionIdx := h.LockedVersion
	if versionIdx <= 0 || versionIdx > len(h.Versions) {
		versionIdx = len(h.Versions) - 1 // latest (0-indexed)
	} else {
		versionIdx-- // convert to 0-based
	}

	b := h.Versions[versionIdx]
	copy := *b
	copy.Schema = deepCopySchema(b.Schema)
	return &copy, true
}

// GetHistory returns the full version history for an endpoint
func (s *Store) GetHistory(method, path string) (*types.EndpointHistory, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", method, path)
	h, exists := s.histories[key]
	if !exists {
		return nil, false
	}

	// Return deep copy
	copy := *h
	copy.Versions = make([]*types.ContractBaseline, len(h.Versions))
	for i, v := range h.Versions {
		vc := *v
		vc.Schema = deepCopySchema(v.Schema)
		copy.Versions[i] = &vc
	}
	return &copy, true
}

// GetAllHistories returns all endpoint histories (copies)
func (s *Store) GetAllHistories() []*types.EndpointHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*types.EndpointHistory, 0, len(s.histories))
	for _, h := range s.histories {
		copy := *h
		copy.Versions = make([]*types.ContractBaseline, len(h.Versions))
		for i, v := range h.Versions {
			vc := *v
			vc.Schema = deepCopySchema(v.Schema)
			copy.Versions[i] = &vc
		}
		list = append(list, &copy)
	}
	return list
}

func (s *Store) GetAllBaselines() []*types.ContractBaseline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*types.ContractBaseline, 0, len(s.histories))
	for _, h := range s.histories {
		if len(h.Versions) > 0 {
			v := h.Versions[len(h.Versions)-1] // latest
			copy := *v
			copy.Schema = deepCopySchema(v.Schema)
			list = append(list, &copy)
		}
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
	h, exists := s.histories[key]
	now := time.Now()

	if !exists {
		h = &types.EndpointHistory{
			Method:           method,
			Path:             path,
			Versions:         make([]*types.ContractBaseline, 0),
			LockedVersion:    0,
			ObservationCount: 0,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		s.histories[key] = h
	}

	// Update frequency-based required keys if this is an object
	if inferredSchema.Type == types.TypeObject && inferredSchema.Properties != nil {
		h.ObservationCount++
		for k := range inferredSchema.Properties {
			// Track field presence frequency
			inferredSchema.Properties[k].SampleValue = nil // we'll track separately
		}
	}

	version := len(h.Versions) + 1
	reqCount := int64(1)
	if len(h.Versions) > 0 {
		reqCount = h.Versions[len(h.Versions)-1].RequestCount + 1
	}

	cb := &types.ContractBaseline{
		ID:            fmt.Sprintf("bl_%d", time.Now().UnixNano()),
		Method:        method,
		Path:          path,
		Schema:        inferredSchema,
		SamplePayload: samplePayload,
		CreatedAt:     now,
		UpdatedAt:     now,
		Version:       version,
		RequestCount:  reqCount,
	}

	if len(h.Versions) > 0 {
		last := h.Versions[len(h.Versions)-1]
		cb.CreatedAt = last.CreatedAt
	}

	h.Versions = append(h.Versions, cb)
	h.UpdatedAt = now

	// Marshal under lock
	var data []byte
	data, err = json.MarshalIndent(s.histories, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	// Atomic write
	if err := atomicWriteFile(s.persistPath, data, 0600); err != nil {
		return nil, fmt.Errorf("atomic write failed: %w", err)
	}
	return cb, nil
}

// SetLockedVersion pins an endpoint to a specific version
func (s *Store) SetLockedVersion(method, path string, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", method, path)
	h, exists := s.histories[key]
	if !exists {
		return fmt.Errorf("endpoint not found")
	}
	if version < 0 || version > len(h.Versions) {
		return fmt.Errorf("invalid version %d (have %d versions)", version, len(h.Versions))
	}
	h.LockedVersion = version
	h.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(s.histories, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.persistPath, data, 0600)
}

func (s *Store) DeleteBaseline(method, path string) {
	s.mu.Lock()
	key := fmt.Sprintf("%s:%s", method, path)
	delete(s.histories, key)

	data, err := json.MarshalIndent(s.histories, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return
	}
	_ = atomicWriteFile(s.persistPath, data, 0600)
}

func (s *Store) GetAlerts(limit int) []types.Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		return []types.Alert{}
	}

	// Determine how many to return
	count := len(s.alertOrder)
	if limit < count {
		count = limit
	}

	// Create result slice
	res := make([]types.Alert, 0, count)

	// Iterate from most recent (end of alertOrder) to least recent
	for i := len(s.alertOrder) - 1; i >= 0 && len(res) < count; i-- {
		key := s.alertOrder[i]
		if alert, exists := s.alerts[key]; exists {
			res = append(res, *alert)
		}
	}

	return res
}

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

func (s *Store) loadHistoriesFromFile() error {
	data, err := os.ReadFile(s.persistPath)
	if err != nil {
		return err // file doesn't exist is OK
	}

	var histories map[string]*types.EndpointHistory
	if err := json.Unmarshal(data, &histories); err != nil {
		// Backup corrupt file
		_ = os.Rename(s.persistPath, s.persistPath+".corrupt."+time.Now().Format("20060102-150405"))
		return err
	}

	s.mu.Lock()
	s.histories = histories
	s.mu.Unlock()
	return nil
}