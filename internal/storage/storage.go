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

// maxObservations bounds the per-endpoint series. It is a ring buffer, not a
// running total: the stability trend shows a window of recent behaviour, and an
// unbounded series would grow once per proxied request for the life of the
// process. The true total lives in ObservationCount.
const maxObservations = 50

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

	s.recordObservationLocked(t)
}

// recordObservationLocked appends one sighting to the endpoint's history. The
// caller must hold s.mu.
//
// This is the join between the two halves of the product. Everything downstream
// of it was previously guesswork: the Version History view had no data by
// construction because Versions only grow when a human accepts a shape,
// ObservationCount counted baseline saves rather than observations, the
// per-endpoint trend had no series to draw, and the lock had nothing to pin
// because it pins an entry in a list that never grew on its own.
func (s *Store) recordObservationLocked(t types.CapturedTraffic) {
	key := fmt.Sprintf("%s:%s", t.Method, t.Path)
	at := t.Timestamp
	if at.IsZero() {
		at = time.Now()
	}

	h, exists := s.histories[key]
	if !exists {
		// An endpoint that has been seen but never baselined still has a
		// history worth showing — that it is unbaselined is itself the finding.
		// Creating the entry does not create a baseline: GetBaseline guards on
		// len(Versions) == 0, not on the map entry existing, so this endpoint
		// reads as unbaselined exactly as before.
		h = &types.EndpointHistory{
			Method:    t.Method,
			Path:      t.Path,
			Versions:  make([]*types.ContractBaseline, 0),
			CreatedAt: at,
			UpdatedAt: at,
		}
		s.histories[key] = h
	}

	h.Observations = append(h.Observations, types.Observation{
		Timestamp:      at,
		StatusCode:     t.StatusCode,
		DurationMs:     t.DurationMs,
		ContractStatus: t.ContractStatus,
	})
	if len(h.Observations) > maxObservations {
		// Copy the tail rather than reslicing forward: reslicing keeps the whole
		// backing array alive and would leak it one entry per request forever.
		// This allocates once per maxObservations requests.
		h.Observations = append([]types.Observation(nil), h.Observations[len(h.Observations)-maxObservations:]...)
	}
	h.ObservationCount++
	h.UpdatedAt = at
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
	// Observations needs its own copy too. Unlike Versions, which only grows
	// when somebody saves a baseline, this slice is appended to on the request
	// path — so a caller handed the backing array is holding memory the store
	// keeps writing into, and a caller who appends to the returned slice writes
	// into the store's series.
	copy.Observations = copyObservations(h.Observations)
	return &copy, true
}

// copyObservations returns an independent copy. Always a non-nil slice, so an
// endpoint with no observations serialises as [] and a client can iterate it
// without a null check.
func copyObservations(in []types.Observation) []types.Observation {
	out := make([]types.Observation, len(in))
	copy(out, in)
	return out
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
		copy.Observations = copyObservations(h.Observations)
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
			Method:        method,
			Path:          path,
			Versions:      make([]*types.ContractBaseline, 0),
			LockedVersion: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		s.histories[key] = h
	}

	// Strip per-field sample values before persisting the schema. They are
	// redundant with SamplePayload, which holds the whole response verbatim, and
	// keeping them would duplicate the payload inside the schema tree for every
	// field at every level. This is not what ObservationCount counted — that
	// increment used to live in this loop, which is why the count moved only
	// when a human saved a baseline.
	if inferredSchema.Type == types.TypeObject && inferredSchema.Properties != nil {
		for k := range inferredSchema.Properties {
			inferredSchema.Properties[k].SampleValue = nil
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
	data, err = json.MarshalIndent(s.historiesForPersistLocked(), "", "  ")
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

	data, err := json.MarshalIndent(s.historiesForPersistLocked(), "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.persistPath, data, 0600)
}

// DeleteBaseline forgets an endpoint's accepted contract, not the endpoint.
//
// It used to delete the whole history entry, which was the same thing while
// entries only existed for baselined endpoints. Now that an entry also holds the
// observation series, deleting it would throw away the traffic measurement too —
// the user asked to forget a contract, not to forget they are serving that path.
// The entry survives with no versions, so the endpoint reads as unbaselined
// again (GetBaseline and GetAllBaselines both guard on len(Versions) == 0) while
// its history stays visible.
func (s *Store) DeleteBaseline(method, path string) {
	s.mu.Lock()
	key := fmt.Sprintf("%s:%s", method, path)
	if h, exists := s.histories[key]; exists {
		h.Versions = make([]*types.ContractBaseline, 0)
		h.LockedVersion = 0
		h.UpdatedAt = time.Now()
	}

	data, err := json.MarshalIndent(s.historiesForPersistLocked(), "", "  ")
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

// historiesForPersistLocked returns the contract state to write to disk, with
// the observation window stripped. The caller must hold s.mu.
//
// Observations are runtime telemetry, not contract: they accumulate once per
// proxied request and are written on baseline saves, so persisting them would
// put an arbitrary snapshot of live traffic into a file that otherwise holds
// nothing but accepted contracts — and would restore a stale window and a stale
// count on restart. In-memory-only is also what s.traffics already does, so the
// two traffic series now agree on their lifetime: a fresh process has observed
// nothing, and says so.
func (s *Store) historiesForPersistLocked() map[string]*types.EndpointHistory {
	out := make(map[string]*types.EndpointHistory, len(s.histories))
	for k, h := range s.histories {
		hc := *h
		hc.Observations = nil
		hc.ObservationCount = 0
		out[k] = &hc
	}
	return out
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