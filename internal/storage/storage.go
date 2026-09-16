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
	configPath   string
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
		configPath:  filepath.Join(persistDir, "config.json"),
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

// UpdateConfig applies a configuration change and writes it to disk, so the
// settings the dashboard offers survive a restart.
//
// It used to mutate memory and nothing else, which meant the target URL and the
// auto-save checkbox silently reverted to their command-line values every time
// the process restarted — a settings panel whose settings were not settings.
func (s *Store) UpdateConfig(cfg types.ProxyConfig) error {
	s.mu.Lock()
	s.config = cfg
	data, err := json.MarshalIndent(cfg, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := atomicWriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

// ApplyRememberedConfig overlays the configuration saved from the dashboard onto
// the store's own, and is called once at startup, before the proxy is built.
//
// The two flags say whether the caller passed --target / --port explicitly. A
// value typed on the command line is a decision about this run and outranks the
// remembered preference; a value left at its default is not a decision at all,
// which is why explicitness is passed in rather than inferred by comparing
// against the default. Without that distinction, launching with the default
// target would silently discard a target the user set in the dashboard.
//
// A missing file is normal and not an error. A corrupt one is reported, moved
// aside, and replaced by the defaults rather than being allowed to prevent
// startup.
func (s *Store) ApplyRememberedConfig(targetFromFlag, portFromFlag bool) error {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var saved types.ProxyConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		_ = os.Rename(s.configPath, s.configPath+".corrupt."+time.Now().Format("20060102-150405"))
		return fmt.Errorf("unreadable saved config, moved aside: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !targetFromFlag && saved.TargetURL != "" {
		s.config.TargetURL = saved.TargetURL
	}
	if !portFromFlag && saved.ProxyPort != "" {
		s.config.ProxyPort = saved.ProxyPort
	}
	// The three booleans are preferences, not command-line values, so the saved
	// ones always apply: there is no flag that could outrank them.
	s.config.AutoSaveBaseline = saved.AutoSaveBaseline
	s.config.InterceptJSON = saved.InterceptJSON
	s.config.DevMockMode = saved.DevMockMode
	return nil
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

// SaveBaseline records a version from a shape a human supplied or accepted.
//
// Callers that know a stronger provenance than "a person meant this" should say
// so through SaveBaselineFrom instead — an imported OpenAPI document is a
// declared contract, and recording it as merely manual would leave the dashboard
// asking the user to confirm something they had already stated.
func (s *Store) SaveBaseline(method, path, samplePayload string) (*types.ContractBaseline, error) {
	return s.SaveBaselineFrom(method, path, samplePayload, types.BaselineSourceManual)
}

// SaveBaselineFrom records a new contract version and states where it came
// from, so a version captured from live traffic can be told apart from one
// somebody vouched for. The schema is inferred from the payload, which is the
// best available answer when the payload is all the evidence there is.
func (s *Store) SaveBaselineFrom(method, path, samplePayload, source string) (*types.ContractBaseline, error) {
	return s.SaveBaselineWithSchema(method, path, samplePayload, nil, source)
}

// SaveBaselineWithSchema records a new contract version whose schema the caller
// already has, falling back to inference when declared is nil.
//
// A declared schema says things a sample cannot. An OpenAPI document states
// which properties are required and what string formats its fields carry, and
// both were being discarded here: this function re-inferred the schema from the
// payload, so an imported contract's `required` list was replaced by "every key
// in the generated example" — every optional property became mandatory, and the
// list the document published was unreadable everywhere downstream. Passing the
// parsed schema through is what makes the import mean what the document said.
func (s *Store) SaveBaselineWithSchema(method, path, samplePayload string, declared *types.JSONSchemaNode, source string) (*types.ContractBaseline, error) {
	s.mu.Lock()

	// The payload is validated even when a schema is supplied: it is stored
	// verbatim as SamplePayload and served to the dashboard, so a malformed one
	// is a corrupt record no matter where the schema came from.
	inferredSchema, err := schema.InferFromJSON(samplePayload)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}
	if declared != nil {
		inferredSchema = declared
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
	//
	// Only for an inferred schema, which this function owns and just built. A
	// declared one belongs to the caller and is stored as given; editing its tree
	// would be a side effect on the spec the caller is still holding.
	if declared == nil && inferredSchema.Type == types.TypeObject && inferredSchema.Properties != nil {
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
		Source:        source,
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

// ConfirmBaseline turns a provisional version into one a human stands behind.
//
// This is the act that closes the auto-save trap. Driftwood cannot know whether
// the first response it ever saw was correct, and when that response is captured
// as the contract, drift that was already present is measured against it and
// comes back MATCH forever. Nothing in the process can detect that; only a
// person can, and only if they are told the contract is a guess. Confirming is
// how they say it is not.
//
// The version must be named explicitly. Zero is not accepted as "the latest":
// confirming a version the caller did not name is how a guess gets blessed by
// someone who was looking somewhere else.
func (s *Store) ConfirmBaseline(method, path string, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", method, path)
	h, exists := s.histories[key]
	if !exists {
		return fmt.Errorf("endpoint not found")
	}
	if version <= 0 || version > len(h.Versions) {
		return fmt.Errorf("invalid version %d (have %d versions)", version, len(h.Versions))
	}

	cb := h.Versions[version-1]
	if !cb.IsProvisional() {
		// Already vouched for, or declared by a spec. Nothing to do, and an error
		// would report a failure for a state the caller asked for and already has.
		return nil
	}
	cb.Source = types.BaselineSourceManual
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