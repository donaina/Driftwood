package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
)

// storeVersion is the format version of the persisted document, written into
// the file so the two formats can be told apart from their contents.
//
// v1 could not be identified at all: it was a bare map of endpoints, so the only
// thing distinguishing "an empty store" from "a store I cannot parse" was the
// absence of keys, and nothing distinguished either from a future format. That
// is why the version is a key of its own rather than, say, inferred from whether
// some field is present.
const storeVersion = 2

// defaultProjectID is the project every install has before projects are a
// user-facing idea, and the one an existing install's endpoints are moved into
// when its store is converted. The id is stable and dull on purpose: it ends up
// in the file, and a migrated install should not look like somebody made a
// project called "Migrated".
const defaultProjectID = "default"

const defaultProjectName = "Default"

/*
Sentinel errors for the project operations, so a caller can tell apart the

	reasons one of them was refused.

	The alternative was to match on the message text, which is the shape of bug
	that makes a 404 depend on someone's wording — and every one of these reaches
	an HTTP status code, where the difference between 400, 404 and 409 is the
	whole of what the caller learns. Wrapped rather than returned bare so the
	existing context in each message survives.
*/
var (
	// ErrNoSuchProject means the id names no project in this store.
	ErrNoSuchProject = errors.New("no such project")
	// ErrProjectNeedsName means a name was empty or only whitespace.
	ErrProjectNeedsName = errors.New("a project needs a name")
	// ErrLastProject means deleting was refused because it would leave none.
	ErrLastProject = errors.New("this is the only project, and Driftwood always has one")
	// ErrTooManyProjects means the install is at maxProjects.
	ErrTooManyProjects = errors.New("too many projects")
)

// persistedState is the whole of what is written to baselines.json.
//
// One document rather than two files, so the project list, the active project
// and the endpoints underneath them cannot disagree with each other: two files
// can be written in either order and a crash between them leaves a store that is
// internally inconsistent, with no way to tell which half is current.
type persistedState struct {
	Version   int                                          `json:"version"`
	Active    string                                       `json:"active_project"`
	Projects  []types.Project                              `json:"projects"`
	Histories map[string]map[string]*types.EndpointHistory `json:"histories"`
}

type Store struct {
	mu sync.RWMutex

	/* Everything a project owns, keyed by project id.

	   Nested rather than a project-prefixed key, so isolation is structural:

	     histories  projectID -> "METHOD:PATH" -> that endpoint's history
	     traffics   projectID -> the traffic ring
	     alertOrder projectID -> the alert traffic IDs raised under it

	   A prefix would make isolation a property of how carefully every key was
	   built. The first key assembled without the prefix lands one project's
	   baseline on top of another's, and nothing about the resulting map says so.

	   The identifiers are used exactly as given, so an id containing a colon is
	   not special. That is the other half of the same argument: with a prefix,
	   "a:b" + ":" + "GET:/x" is a key another project can also produce. */
	histories  map[string]map[string]*types.EndpointHistory
	traffics   map[string][]types.CapturedTraffic
	alertOrder map[string][]string

	/* Alerts are keyed by traffic ID rather than by project.

	   A traffic ID is unique across the whole store, so a project dimension here
	   would carry no information — and would have to be kept in step with
	   traffic's for no benefit. alertOrder is the one that needs a project,
	   because ordering is the whole of what it provides. */
	alerts map[string]*types.Alert

	config      types.ProxyConfig
	maxTraffics int
	maxAlerts   int
	persistPath string
	configPath  string

	/* The projects this install knows about, and which one is current.

	   Held as a map plus an order slice rather than a slice alone because the
	   document is written from it and a map iterates in a random order — a slice
	   built by ranging a map would rewrite the project list differently on every
	   save, so an unchanged store would show as a diff every time. */
	projects     map[string]*types.Project
	projectOrder []string
	active       string

	/* Serialises the rename-into-place writes, and must be held across the whole
	   mutate-then-snapshot-then-write sequence rather than just the write.

	   Held around the write alone it does nothing useful: two saves can each take
	   their snapshot under mu, then write in the opposite order, and the file ends
	   up holding the older snapshot. The file stays well-formed and a whole
	   baseline disappears, which is the failure mode worth naming — it does not
	   look like corruption, so nothing downstream reports it.

	   Lock order is writeMx before mu. Every persisting method acquires them in
	   that order; none acquires mu first and then waits for writeMx, so the two
	   cannot deadlock. Holding mu across the mutation is still what makes the
	   snapshot consistent; writeMx is what makes the file's history match the
	   order the mutations happened in. */
	writeMx sync.Mutex

	/* Whether anyone has ever told this install what to sniff: a saved config,
	   or --target/--port on the command line.

	   This exists because the dashboard needs to know whether to offer its
	   first-run setup, and it used to infer that from the store holding zero
	   baselines. That inference was wrong twice over. Driftwood seeds a contract
	   for its own mock simulator, so a fresh install always had one; and every
	   request the proxy sees is auto-baselined, so a browser's incidental
	   GET /favicon.ico was enough to make an untouched install look configured.
	   Neither is the operator saying anything. This is. */
	configured bool

	/* routingGen counts every change to where requests go: which project is
	   active, which projects exist, and what any of them targets.

	   It exists so the proxy cannot route by a stale snapshot. The proxy holds a
	   copy for the request path to read without a lock, and a copy that has to be
	   told when to refresh is a copy that will eventually not be told — the
	   symptom being requests delivered to the previous client's backend after a
	   switch, which is precisely the misattribution this codebase goes out of its
	   way to avoid. Comparing this counter is a second atomic load on the request
	   path and makes the stale case unrepresentable rather than unlikely.

	   Bumped explicitly by the methods that change routing, not by every persist:
	   a baseline save persists, and doing it there would rebuild the snapshot once
	   per proxied request. */
	routingGen atomic.Uint64
}

// markRoutingChangedLocked records that where requests go has changed. The
// caller must hold s.mu, so the bump and the change it describes are ordered
// together — a proxy that reads the new generation is guaranteed to see the
// change that produced it.
func (s *Store) markRoutingChangedLocked() {
	s.routingGen.Add(1)
}

// RoutingGeneration reports how many times routing has changed. The proxy
// compares it against the generation its snapshot was built from; see the field
// comment for why this is a counter rather than a notification.
func (s *Store) RoutingGeneration() uint64 {
	return s.routingGen.Load()
}

// NewStore builds a store and loads whatever was saved under the home
// directory.
//
// The error reports something the operator needs to know — a saved store that
// could not be read, or one that was converted from an older format — and not a
// failure to produce a store. The store returned alongside it is always usable,
// and the caller is expected to log and carry on: refusing to start because the
// contracts on disk could not be parsed would turn a recoverable problem into an
// outage, and the endpoints are re-learned from traffic anyway. What must not
// happen is the v1 behaviour, which was to discard this error with `_ =` and
// start empty without mentioning it.
func NewStore(targetURL, proxyPort string) (*Store, error) {
	homeDir, _ := os.UserHomeDir()
	persistDir := filepath.Join(homeDir, ".driftwood")
	_ = os.MkdirAll(persistDir, 0700)

	now := time.Now()
	s := &Store{
		alerts:      make(map[string]*types.Alert),
		maxTraffics: 500,
		maxAlerts:   200,
		persistPath: filepath.Join(persistDir, "baselines.json"),
		configPath:  filepath.Join(persistDir, "config.json"),
		projects: map[string]*types.Project{
			defaultProjectID: {
				ID:   defaultProjectID,
				Name: defaultProjectName,
				// The flag's target is the first project's target. It is seeded here
				// and then persisted, so this is the one time the value comes from
				// the command line rather than from the document.
				//
				// AllowPrivate is set here without a loopback check, and that is the
				// existing rule rather than a new exception: the constructor's target
				// came from the operator typing a flag, and an operator naming
				// http://localhost:3000 is naming the thing they want sniffed. The
				// check applies to targets that arrive over the wire, which is what
				// SetProjectTarget's allowPrivate argument records.
				TargetURL:          targetURL,
				TargetAllowPrivate: true,
				CreatedAt:          now,
			},
		},
		projectOrder: []string{defaultProjectID},
		active:       defaultProjectID,
		config: types.ProxyConfig{
			TargetURL:        targetURL,
			ProxyPort:        proxyPort,
			AutoSaveBaseline: true,
			InterceptJSON:    true,
		},
	}
	s.ensureProjectLocked(defaultProjectID)

	// Nothing else can see the store yet, so the load assigns its fields directly
	// rather than taking the lock a shared store would need.
	return s, s.loadFromFile()
}

// historyKey is how an endpoint is identified everywhere in this package: one
// map key per METHOD:PATH, built here and nowhere else.
//
// It was inlined at seven call sites, which is seven chances for the read path
// and the write path to disagree about how an endpoint is spelled. A key that
// one function writes as "GET:/users" and another looks up as "GET /users" is
// not a compile error and not a runtime error either — it is an endpoint that
// silently never matches its own baseline, which reads as drift that is not
// there, or as a contract that vanished.
//
// The Method is used exactly as given, so the key is case-sensitive and nothing
// upstream normalises it: net/http hands over r.Method as the client spelled it,
// and `curl -X get` therefore files its history under a second key. Real clients
// send the method upper-case, so this is rare rather than theoretical. It is
// left as-is because a key upper-cased at lookup time would not match a
// lower-case key already sitting in a user's baselines.json — normalising means
// normalising on load too, which belongs with the keying rewrite and not here.
func historyKey(method, path string) string {
	return method + ":" + path
}

// SetConfigured records that the operator has named a target or a port, which
// the dashboard reads back to decide whether to offer first-run setup.
func (s *Store) SetConfigured(configured bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configured = configured
}

// IsConfigured reports whether this install has ever been pointed at anything.
func (s *Store) IsConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configured
}

// GetConfig returns the running configuration, with the target read from the
// active project.
//
// The project is the one place a target is stored. The config used to hold one
// too, which made two answers to "what are we sniffing" — and the moment a
// project could be switched, the config's copy would have been the one every
// reader saw while the proxy dialled the other. Overlaying on the way out means
// the dashboard, the setup wizard and the proxy all see the same value without
// any of them having to know projects exist.
func (s *Store) GetConfig() types.ProxyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.config
	if p, ok := s.projects[s.active]; ok {
		cfg.TargetURL = p.TargetURL
	}
	return cfg
}

// ActiveProject is the project a caller with no opinion should read and write.
//
// It exists so that "which project is this" has one answer in one place. The
// alternative — each accessor deciding for itself — is how two views of the same
// data end up disagreeing about which project is on screen.
//
// Nothing here resolves the active project on a caller's behalf. Every data
// accessor takes a project id, so a caller that means "the one the user is
// looking at" says so by passing this, and a caller that means a specific
// project passes that — and neither the store nor the accessor has to guess.
func (s *Store) ActiveProject() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// ensureProjectLocked makes sure the maps a project needs exist, and returns
// them. The caller must hold s.mu.
//
// Lazy rather than eager because a project is created in three places — the
// first-run default, a load from disk, and an explicit create — and only the
// last of them has any reason to allocate empty rings for a project nobody has
// sent traffic to yet. It also means a Store assembled by hand in a test is
// usable, which is why the accessors call this rather than assuming the maps
// were built for them.
//
// It does not touch s.projects. Registering a project and having somewhere to
// put its data are separate: a read for a project that does not exist must
// answer "nothing", not conjure the project into being.
func (s *Store) ensureProjectLocked(projectID string) {
	if s.histories == nil {
		s.histories = make(map[string]map[string]*types.EndpointHistory)
	}
	if s.traffics == nil {
		s.traffics = make(map[string][]types.CapturedTraffic)
	}
	if s.alertOrder == nil {
		s.alertOrder = make(map[string][]string)
	}
	if _, ok := s.histories[projectID]; !ok {
		s.histories[projectID] = make(map[string]*types.EndpointHistory)
	}
	if _, ok := s.traffics[projectID]; !ok {
		s.traffics[projectID] = make([]types.CapturedTraffic, 0)
	}
	if _, ok := s.alertOrder[projectID]; !ok {
		s.alertOrder[projectID] = make([]string, 0)
	}
}

// ProjectExists reports whether this id names a project the store knows about.
func (s *Store) ProjectExists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.projects[id]
	return ok
}

// ListProjects returns every project in creation order, and which one is active.
// GetProject returns one project by id.
//
// It exists because writing a project's target does not update a copy the caller
// already holds: CreateProject returns a value, SetProjectTarget mutates the
// store's own, and a handler that returned the first would report a project it
// had just configured as having no backend.
func (s *Store) GetProject(id string) (types.Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.projects[id]
	if !ok {
		return types.Project{}, false
	}
	return *p, true
}

func (s *Store) ListProjects() ([]types.Project, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]types.Project, 0, len(s.projectOrder))
	for _, id := range s.projectOrder {
		if p, ok := s.projects[id]; ok {
			out = append(out, *p)
		}
	}
	return out, s.active
}

// CreateProject registers a new project and returns it.
//
// The id is derived from the name and then made unique, rather than taken from
// the caller. Callers are HTTP handlers and command-line arguments, and an id
// that a caller chooses is an id a caller can choose badly — a duplicate, an
// empty string, one that collides with the migrated "default". Deriving it means
// the id is always well-formed and always free.
func (s *Store) CreateProject(name string) (*types.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrProjectNeedsName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.projects) >= maxProjects {
		return nil, fmt.Errorf("%w: this install is at its limit of %d", ErrTooManyProjects, maxProjects)
	}

	p := &types.Project{
		ID:        s.uniqueProjectIDLocked(name),
		Name:      name,
		CreatedAt: time.Now(),
	}
	s.projects[p.ID] = p
	s.projectOrder = append(s.projectOrder, p.ID)
	s.ensureProjectLocked(p.ID)
	s.markRoutingChangedLocked()

	out := *p
	return &out, nil
}

// uniqueProjectIDLocked turns a name into a free id. The caller must hold s.mu.
//
// The suffix starts at 2 because "acme" and "acme-2" both exist as plausible
// first choices, and a second project called "Acme" becoming "acme-2" is what a
// person would expect; there is no "acme-1" anywhere to explain.
func (s *Store) uniqueProjectIDLocked(name string) string {
	base := slugify(name)
	if base == "" {
		base = "project"
	}
	if _, taken := s.projects[base]; !taken {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if _, taken := s.projects[candidate]; !taken {
			return candidate
		}
	}
}

// slugify reduces a name to something safe to use as an identifier and to put
// in a URL.
//
// Deliberately lossy: anything that is not a letter, a digit or a dash becomes a
// dash, so the result is always usable as an id without escaping. The name is
// kept separately and shown to the user, so nothing here is the display name —
// which is what makes it acceptable to throw information away.
func slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			// Collapse runs, and never lead with a dash: "  Acme  Ltd " should
			// become "acme-ltd", not "-acme--ltd-".
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// RenameProject changes a project's display name. The id is not affected: ids
// are in the document, in alert records and in any URL a user has kept, so
// renaming something should not move it.
func (s *Store) RenameProject(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrProjectNeedsName
	}

	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNoSuchProject, id)
	}
	p.Name = name
	s.mu.Unlock()

	return s.persistLocked()
}

// SetProjectTarget records the backend a project is sniffing.
//
// allowPrivate is stored rather than recomputed, and it is the caller's job to
// have established it honestly — this method does not check where the request
// came from, because the store has no idea what a request is. The proxy is what
// decides, at the point the target arrives, and this is the record of that
// decision. Splitting it that way keeps the SSRF rule in one place instead of
// two that could disagree.
func (s *Store) SetProjectTarget(id, targetURL string, allowPrivate bool) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNoSuchProject, id)
	}
	p.TargetURL = targetURL
	p.TargetAllowPrivate = allowPrivate
	s.markRoutingChangedLocked()
	s.mu.Unlock()

	return s.persistLocked()
}

// ProjectTarget returns a project's backend and whether it was authorised to be
// a private one. The bool is false for a project that does not exist, which is
// the same answer as a project with no target: both mean "nothing to dial".
func (s *Store) ProjectTarget(id string) (string, bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.projects[id]
	if !ok {
		return "", false, false
	}
	return p.TargetURL, p.TargetAllowPrivate, true
}

// Routing returns what the proxy needs to resolve a request: which project is
// active, and every project's target.
//
// One call under one lock rather than a series of accessors, because the proxy
// snapshots this into a single atomic value and a snapshot assembled from
// separate calls could catch the store mid-switch — an active project from
// before a change paired with targets from after it.
// Routing returns where requests should currently go, together with the
// generation those answers were read at.
//
// The generation comes back from inside the read lock rather than from a
// separate call to RoutingGeneration, and that is the whole point of returning
// it here. Read apart, a mutation can land between the two: the caller takes its
// content before the change and its generation after it, stamps a stale snapshot
// as current, and the staleness this counter exists to detect is exactly what it
// would hide. Read together, a stamped generation describes the content it is
// stamped on, and any later mutation is guaranteed to bump past it.
func (s *Store) Routing() (string, map[string]TargetDecision, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targets := make(map[string]TargetDecision, len(s.projects))
	for id, p := range s.projects {
		targets[id] = TargetDecision{URL: p.TargetURL, AllowPrivate: p.TargetAllowPrivate}
	}
	return s.active, targets, s.routingGen.Load()
}

// TargetDecision is a project's backend together with the authorisation that was
// recorded when it was set.
type TargetDecision struct {
	URL          string
	AllowPrivate bool
}

// SetActiveProject points the install at one of its projects.
//
// Persisted, because the active project is a decision the operator made rather
// than a property of the process: a restart that silently resurfaced a different
// client's contracts would be the kind of quiet misattribution this codebase
// goes out of its way to avoid.
func (s *Store) SetActiveProject(id string) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	if _, ok := s.projects[id]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNoSuchProject, id)
	}
	s.active = id
	s.ensureProjectLocked(id)
	s.markRoutingChangedLocked()
	s.mu.Unlock()

	return s.persistLocked()
}

// DeleteProject removes a project and everything recorded under it.
//
// It refuses to remove the last one. An install with no projects has no active
// project, and every accessor is written against one existing — so the state is
// not merely empty, it is one the rest of the store does not describe. Refusing
// is also the honest answer: the user asked to delete a project, and there is
// always something else to be looking at instead.
func (s *Store) DeleteProject(id string) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	if _, ok := s.projects[id]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNoSuchProject, id)
	}
	if len(s.projects) <= 1 {
		s.mu.Unlock()
		return ErrLastProject
	}

	// The alerts first. They are keyed by traffic ID rather than by project, so
	// nothing else would collect them: without this the map keeps one entry per
	// alert the project ever raised, reachable from nowhere, for the life of the
	// process. Deleting a project is rare and this is the only place the
	// dimension has to be reconstructed by hand.
	for _, trafficID := range s.alertOrder[id] {
		delete(s.alerts, trafficID)
	}
	delete(s.alertOrder, id)
	delete(s.histories, id)
	delete(s.traffics, id)
	delete(s.projects, id)

	for i, existing := range s.projectOrder {
		if existing == id {
			s.projectOrder = append(s.projectOrder[:i:i], s.projectOrder[i+1:]...)
			break
		}
	}
	// Only when the deleted project held it. Picking a new active project while
	// the current one is still present would move the user's view as a side
	// effect of deleting something else.
	if s.active == id {
		s.active = s.projectOrder[0]
	}
	s.markRoutingChangedLocked()
	s.mu.Unlock()

	return s.persistLocked()
}

// UpdateConfig applies a configuration change and writes it to disk, so the
// settings the dashboard offers survive a restart.
//
// It used to mutate memory and nothing else, which meant the target URL and the
// auto-save checkbox silently reverted to their command-line values every time
// the process restarted — a settings panel whose settings were not settings.
//
// A target in cfg is written to the active project, and config.json keeps it as
// well. That is deliberate rather than the duplication it looks like: the two
// files answer different questions. config.json is what this install was last
// told to do, which is what ApplyRememberedConfig reads on the next start; the
// project's copy is what is in force, and is what a switch changes. They agree
// for a single-project install, which is the only install there has ever been,
// and a second project diverging from the remembered default is the point of
// having one.
func (s *Store) UpdateConfig(cfg types.ProxyConfig) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	if p, ok := s.projects[s.active]; ok {
		p.TargetURL = cfg.TargetURL
	}
	s.markRoutingChangedLocked()
	s.config = cfg
	s.configured = true
	data, err := json.MarshalIndent(cfg, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := atomicWriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	// The project's target lives in the store document, not in config.json, so
	// the write above does not carry it. Without this the retarget would survive
	// a restart only by way of the remembered config, and would be lost by a
	// launch that passed --target.
	return s.persistLocked()
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
		// And onto the project, which is where the proxy reads it from. Writing
		// only the config would leave the remembered target visible to every
		// reader of GetConfig while the request path kept dialling the flag's —
		// the same class of disagreement GetConfig's overlay exists to prevent,
		// arriving from the other direction.
		if p, ok := s.projects[s.active]; ok {
			p.TargetURL = saved.TargetURL
			s.markRoutingChangedLocked()
		}
	}
	if !portFromFlag && saved.ProxyPort != "" {
		s.config.ProxyPort = saved.ProxyPort
	}
	// The three booleans are preferences, not command-line values, so the saved
	// ones always apply: there is no flag that could outrank them.
	s.config.AutoSaveBaseline = saved.AutoSaveBaseline
	s.config.InterceptJSON = saved.InterceptJSON
	s.config.DevMockMode = saved.DevMockMode
	// A config on disk is a config somebody saved: the setup wizard, or the
	// settings panel, or an earlier run. Either way the install is set up.
	s.configured = true
	return nil
}

// maxObservations bounds the per-endpoint series. It is a ring buffer, not a
// running total: the stability trend shows a window of recent behaviour, and an
// unbounded series would grow once per proxied request for the life of the
// process. The true total lives in ObservationCount.
const maxObservations = 50

// maxEndpoints bounds how many endpoints are held in memory.
//
// One entry is created per distinct METHOD:PATH observed, so a path carrying a
// parameter — /orders/{id}, /users/abc123 — creates one per entity, and nothing
// ever removed one. Each entry can hold a baseline whose SamplePayload is a
// whole response body, so this was not a table of counters growing by a row per
// endpoint: it was growing by a response per endpoint, without limit, for the
// life of the process.
const maxEndpoints = 200

// maxProjects bounds how many projects one install can hold.
//
// Every other cap here is per project, which bounds each project and not the
// install: twenty projects each holding 200 endpoints, 500 traffic records and
// 200 alerts is twenty times the memory of one, and nothing but a person
// clicking "new project" can get there. Stating the number is what makes the
// worst case a fact rather than an accident.
//
// Deliberately small. This is not multi-tenancy and does not pretend to be —
// there is no identity in the system, and the projects belong to one operator
// working across a handful of clients. A limit that is comfortable to reach by
// hand is the right shape for a limit whose purpose is to be a backstop.
const maxProjects = 20

// makeRoomForEndpointLocked frees a slot if the project's history map is at its
// cap. The caller must hold s.mu.
//
// Per project, and that is the whole point of it taking a project: a single
// global map with a project field on each entry would be capped globally, so one
// busy client could evict another client's contracts. That is not isolation with
// a rough edge — it is isolation that does not hold, because the eviction is
// driven by traffic the other project never sent.
//
// A map already over the cap — one written by an older build and loaded from
// disk — is left at its size and simply stops growing. Draining it would be the
// cap working as intended and also a silent deletion of records the user can
// currently see, and growth is the part that runs away.
func (s *Store) makeRoomForEndpointLocked(projectID string) {
	if len(s.histories[projectID]) < maxEndpoints {
		return
	}
	s.evictOneEndpointLocked(projectID)
}

// evictOneEndpointLocked drops the endpoint that has gone longest without being
// seen, and reports whether it found one it was free to drop.
//
// Only an endpoint Driftwood is free to forget is a candidate: one that is not
// pinned to a version and none of whose versions a human confirmed. That guard
// is the point of the function. Dropping a confirmed baseline does not merely
// lose a record — the endpoint becomes unbaselined, the next response to arrive
// is auto-captured as its contract, and a shape somebody accepted is silently
// replaced by one nobody has looked at. That is precisely the failure this
// product exists to catch, so it must not be the price of a memory cap.
//
// It follows that an endpoint with nothing to lose is a candidate whatever its
// history, and a pinned or confirmed one never is. Should every endpoint be
// pinned or confirmed, no slot is freed and the map is allowed past the cap:
// those entries can only be created deliberately, one at a time, by a person,
// which is a bound that traffic cannot drive past.
func (s *Store) evictOneEndpointLocked(projectID string) bool {
	histories := s.histories[projectID]
	victim := ""
	var victimAt time.Time
	for key, h := range histories {
		if h.LockedVersion != 0 || hasConfirmedVersion(h) {
			continue
		}
		if victim == "" || h.UpdatedAt.Before(victimAt) {
			victim, victimAt = key, h.UpdatedAt
		}
	}
	if victim == "" {
		return false
	}
	delete(histories, victim)
	return true
}

// hasConfirmedVersion reports whether any version of this endpoint came from
// something other than live traffic — see ContractBaseline.IsProvisional.
func hasConfirmedVersion(h *types.EndpointHistory) bool {
	for _, v := range h.Versions {
		if !v.IsProvisional() {
			return true
		}
	}
	return false
}

func (s *Store) AddTraffic(projectID string, t types.CapturedTraffic) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureProjectLocked(projectID)

	// Stamped here rather than left to the caller, so the record says which
	// project produced it even though the caller already had to name one. The
	// alternative is a record whose project is only knowable from which list it
	// was found in — which stops being true the moment two lists are on screen
	// at once, and is what the traffic view would have had to reconstruct.
	t.ProjectID = projectID

	traffics := append([]types.CapturedTraffic{t}, s.traffics[projectID]...)
	if len(traffics) > s.maxTraffics {
		traffics = traffics[:s.maxTraffics]
	}
	s.traffics[projectID] = traffics

	if t.Diff != nil && (t.Diff.HasBreakingChanges || t.Diff.HasWarnings) {
		alert := &types.Alert{
			TrafficID:      t.ID,
			Endpoint:       fmt.Sprintf("%s %s", t.Method, t.Path),
			ContractStatus: t.ContractStatus,
			Diff:           t.Diff,
			AIExplanation:  nil,
		}
		s.alerts[t.ID] = alert
		order := append(s.alertOrder[projectID], t.ID)
		if len(order) > s.maxAlerts {
			delete(s.alerts, order[0])
			// Copy the tail rather than reslicing forward, for the reason spelled
			// out in recordObservationLocked: `order[1:]` keeps the whole
			// backing array alive, so the evicted traffic ID stays reachable and
			// the array never grows back down. This allocates once per alert past
			// the cap, and alerts are only raised for a breaking change or a
			// warning, so that is a rare allocation rather than a per-request one.
			order = append([]string(nil), order[1:]...)
		}
		s.alertOrder[projectID] = order
	}

	s.recordObservationLocked(projectID, t)
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
func (s *Store) recordObservationLocked(projectID string, t types.CapturedTraffic) {
	key := historyKey(t.Method, t.Path)
	at := t.Timestamp
	if at.IsZero() {
		at = time.Now()
	}

	h, exists := s.histories[projectID][key]
	if !exists {
		// An endpoint that has been seen but never baselined still has a
		// history worth showing — that it is unbaselined is itself the finding.
		// Creating the entry does not create a baseline: GetBaseline guards on
		// len(Versions) == 0, not on the map entry existing, so this endpoint
		// reads as unbaselined exactly as before.
		s.makeRoomForEndpointLocked(projectID)
		h = &types.EndpointHistory{
			Method:    t.Method,
			Path:      t.Path,
			Versions:  make([]*types.ContractBaseline, 0),
			CreatedAt: at,
			UpdatedAt: at,
		}
		s.histories[projectID][key] = h
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

func (s *Store) GetTraffics(projectID string, limit int) []types.CapturedTraffic {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ring := s.traffics[projectID]
	if limit <= 0 || limit > len(ring) {
		limit = len(ring)
	}
	result := make([]types.CapturedTraffic, limit)
	copy(result, ring[:limit])
	return result
}

// ClearTraffic empties one project's traffic ring and leaves every other
// project's alone. It is the dashboard's clear button, and clearing the view you
// are looking at must not clear the ones you are not.
func (s *Store) ClearTraffic(projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureProjectLocked(projectID)
	s.traffics[projectID] = make([]types.CapturedTraffic, 0)
}

// GetBaseline returns a COPY of the latest (or locked) version
func (s *Store) GetBaseline(projectID, method, path string) (*types.ContractBaseline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := historyKey(method, path)
	h, exists := s.histories[projectID][key]
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
func (s *Store) GetHistory(projectID, method, path string) (*types.EndpointHistory, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := historyKey(method, path)
	h, exists := s.histories[projectID][key]
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

// GetAllHistories returns every endpoint history a project holds (copies).
//
// One project's, not the install's. It used to be the union of everything, and
// every caller wanted a single project's data — so the union was a shape no
// screen could use, and the first caller to render it would have shown one
// client's endpoints under another client's name.
func (s *Store) GetAllHistories(projectID string) []*types.EndpointHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	histories := s.histories[projectID]
	list := make([]*types.EndpointHistory, 0, len(histories))
	for _, h := range histories {
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

func (s *Store) GetAllBaselines(projectID string) []*types.ContractBaseline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	histories := s.histories[projectID]
	list := make([]*types.ContractBaseline, 0, len(histories))
	for _, h := range histories {
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
func (s *Store) SaveBaseline(projectID, method, path, samplePayload string) (*types.ContractBaseline, error) {
	return s.SaveBaselineFrom(projectID, method, path, samplePayload, types.BaselineSourceManual)
}

// SaveBaselineFrom records a new contract version and states where it came
// from, so a version captured from live traffic can be told apart from one
// somebody vouched for. The schema is inferred from the payload, which is the
// best available answer when the payload is all the evidence there is.
func (s *Store) SaveBaselineFrom(projectID, method, path, samplePayload, source string) (*types.ContractBaseline, error) {
	return s.SaveBaselineWithSchema(projectID, method, path, samplePayload, nil, source)
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
func (s *Store) SaveBaselineWithSchema(projectID, method, path, samplePayload string, declared *types.JSONSchemaNode, source string) (*types.ContractBaseline, error) {
	// Held across the mutation and the write together: see the note on writeMx.
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	s.ensureProjectLocked(projectID)

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

	key := historyKey(method, path)
	h, exists := s.histories[projectID][key]
	now := time.Now()

	if !exists {
		s.makeRoomForEndpointLocked(projectID)
		h = &types.EndpointHistory{
			Method:        method,
			Path:          path,
			Versions:      make([]*types.ContractBaseline, 0),
			LockedVersion: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		s.histories[projectID][key] = h
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
	s.mu.Unlock()

	if err := s.persistLocked(); err != nil {
		return nil, err
	}

	// A copy, not cb itself. cb is now in s.histories, so returning it hands the
	// caller a live handle on store state that the store's lock does not guard:
	// whoever holds it can rewrite a field from outside any critical section, and
	// a concurrent reader sees the change with no happens-before edge. Every
	// getter in this file copies for exactly that reason; this function, which
	// both mutates and returns, was the one that did not.
	out := *cb
	out.Schema = deepCopySchema(cb.Schema)
	return &out, nil
}

// SetLockedVersion pins an endpoint to a specific version
func (s *Store) SetLockedVersion(projectID, method, path string, version int) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	key := historyKey(method, path)
	h, exists := s.histories[projectID][key]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("endpoint not found")
	}
	if version < 0 || version > len(h.Versions) {
		s.mu.Unlock()
		return fmt.Errorf("invalid version %d (have %d versions)", version, len(h.Versions))
	}
	h.LockedVersion = version
	h.UpdatedAt = time.Now()
	s.mu.Unlock()

	return s.persistLocked()
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
func (s *Store) ConfirmBaseline(projectID, method, path string, version int) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	key := historyKey(method, path)
	h, exists := s.histories[projectID][key]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("endpoint not found")
	}
	if version <= 0 || version > len(h.Versions) {
		s.mu.Unlock()
		return fmt.Errorf("invalid version %d (have %d versions)", version, len(h.Versions))
	}

	cb := h.Versions[version-1]
	if !cb.IsProvisional() {
		// Already vouched for, or declared by a spec. Nothing to do, and an error
		// would report a failure for a state the caller asked for and already has.
		s.mu.Unlock()
		return nil
	}
	cb.Source = types.BaselineSourceManual
	h.UpdatedAt = time.Now()
	s.mu.Unlock()

	return s.persistLocked()
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
//
// It reports a failed write. It used to discard one, with `_ = atomicWriteFile`,
// so a delete that never reached disk looked exactly like one that did — and the
// contract came back on the next restart with nothing on screen or in the log to
// explain why.
func (s *Store) DeleteBaseline(projectID, method, path string) error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	s.mu.Lock()
	key := historyKey(method, path)
	if h, exists := s.histories[projectID][key]; exists {
		h.Versions = make([]*types.ContractBaseline, 0)
		h.LockedVersion = 0
		h.UpdatedAt = time.Now()
	}
	s.mu.Unlock()

	return s.persistLocked()
}

func (s *Store) GetAlerts(projectID string, limit int) []types.Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		return []types.Alert{}
	}

	order := s.alertOrder[projectID]

	// Determine how many to return
	count := len(order)
	if limit < count {
		count = limit
	}

	// Create result slice
	res := make([]types.Alert, 0, count)

	// Iterate from most recent (end of the order) to least recent
	for i := len(order) - 1; i >= 0 && len(res) < count; i-- {
		key := order[i]
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
func (s *Store) historiesForPersistLocked(projectID string) map[string]*types.EndpointHistory {
	histories := s.histories[projectID]
	out := make(map[string]*types.EndpointHistory, len(histories))
	for k, h := range histories {
		hc := *h
		hc.Observations = nil
		hc.ObservationCount = 0
		out[k] = &hc
	}
	return out
}

// stateForPersistLocked assembles the document to write. The caller must hold
// s.mu.
//
// Projects are emitted in projectOrder rather than by ranging the map, so an
// unchanged store writes byte-identical bytes: ranging a map would reorder the
// list on every save and make every write look like a change.
func (s *Store) stateForPersistLocked() *persistedState {
	projects := make([]types.Project, 0, len(s.projectOrder))
	for _, id := range s.projectOrder {
		if p, ok := s.projects[id]; ok {
			projects = append(projects, *p)
		}
	}

	// An install always has a project. NewStore seeds the first one, loadFromFile
	// repairs a document that lists none, and DeleteProject refuses to remove the
	// last. This is the remaining case — a document whose active project is not
	// in its own list — and picking the first real one is recoverable where
	// writing a dangling id is not.
	active := s.active
	if _, ok := s.projects[active]; !ok && len(projects) > 0 {
		active = projects[0].ID
	}

	// Every project's contracts, not only the active one's. Switching project
	// must not be the moment a client's baselines are written down for the first
	// time: a store that only ever persisted what was on screen would lose
	// whatever the other projects had learned the moment the process stopped.
	//
	// Emitted in projectOrder for the same reason the project list is, so the
	// document does not reshuffle itself on every save.
	histories := make(map[string]map[string]*types.EndpointHistory, len(s.projectOrder))
	for _, id := range s.projectOrder {
		if _, ok := s.projects[id]; ok {
			histories[id] = s.historiesForPersistLocked(id)
		}
	}

	return &persistedState{
		Version:   storeVersion,
		Active:    active,
		Projects:  projects,
		Histories: histories,
	}
}

// persistLocked writes the current state. The caller must hold writeMx; it takes
// mu itself for the snapshot, so a caller must not be holding mu already.
func (s *Store) persistLocked() error {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.stateForPersistLocked(), "", "  ")
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	if err := atomicWriteFile(s.persistPath, data, 0600); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	return nil
}

// atomicWriteFile writes path by creating a sibling temp file and renaming it
// into place, so a reader never sees a half-written file.
//
// The temp name is derived from the destination. It used to be the constant
// ".baselines.*.tmp" for every caller, which was invisible while baselines.json
// was the only file written through here and wrong the moment config.json was:
// a leftover temp file named for the wrong file is a confusing thing to find in
// ~/.driftwood, and the pattern is the only clue to which write was interrupted.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
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

// loadFromFile reads the persisted store, converting an older document on the
// way through. A missing file is a fresh install and not an error.
//
// It is the only place that reads this file, and it is deliberately the only
// place that decides what format it is in — a second reader that guessed
// differently would reintroduce exactly the problem the version key exists to
// remove.
func (s *Store) loadFromFile() error {
	data, err := os.ReadFile(s.persistPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not read %s: %w", s.persistPath, err)
	}

	state, converted, err := decodeStore(data)
	if err != nil {
		// Moved aside rather than deleted, and the rename's own failure is
		// reported rather than discarded: a corrupt store that could not be moved
		// is still sitting where the next write will overwrite it, and that is a
		// different situation from one safely out of the way.
		aside := s.persistPath + ".corrupt." + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(s.persistPath, aside); renameErr != nil {
			return fmt.Errorf("%s is unreadable (%v) and could not be moved aside (%v), so it has been left in place and will be overwritten by the next save", s.persistPath, err, renameErr)
		}
		return fmt.Errorf("%s is unreadable and was moved to %s; starting with no saved contracts: %v", s.persistPath, aside, err)
	}

	s.projects = make(map[string]*types.Project, len(state.Projects))
	s.projectOrder = make([]string, 0, len(state.Projects))
	for i := range state.Projects {
		p := state.Projects[i]
		s.projects[p.ID] = &p
		s.projectOrder = append(s.projectOrder, p.ID)
	}
	s.active = state.Active
	s.markRoutingChangedLocked()
	if _, ok := s.projects[s.active]; !ok {
		// A document naming an active project it does not list is internally
		// inconsistent, and picking the first project is recoverable where
		// refusing to start is not.
		if len(s.projectOrder) > 0 {
			s.active = s.projectOrder[0]
		} else {
			s.active = defaultProjectID
			s.projects[defaultProjectID] = &types.Project{
				ID:        defaultProjectID,
				Name:      defaultProjectName,
				CreatedAt: time.Now(),
			}
			s.projectOrder = []string{defaultProjectID}
		}
	}

	// Every project's histories, not only the active one's. A load that kept the
	// active project's endpoints and dropped the rest would make switching
	// project a destructive act: the other clients' contracts would still be in
	// the file, and the first save after the switch would write the document
	// from a store that no longer had them.
	//
	// A project listed in the document with no histories under it gets an empty
	// map rather than a nil one, so every project the store reports is one the
	// accessors can be pointed at without an existence check first.
	s.histories = make(map[string]map[string]*types.EndpointHistory, len(s.projectOrder))
	for _, id := range s.projectOrder {
		histories, ok := state.Histories[id]
		if !ok {
			histories = make(map[string]*types.EndpointHistory)
		}
		s.histories[id] = histories
	}

	if converted {
		return s.keepV1CopyThenRewrite(data)
	}
	return nil
}

// decodeStore reads either document format and always answers with a current
// one. The bool reports whether the bytes were v1 and were converted.
//
// The two formats are told apart by the presence of the "version" key, not by
// decoding into persistedState and checking whether Version came back zero.
// Zero is a meaningful value here, so "decoded as zero" does not mean "was not
// there": a store carrying an explicit version 0 would be read as v1 by that
// shortcut, and the check would be answering a question about the parsed value
// when the question is about which format the bytes are in.
//
// The shortcut is also wrong for the more common case of a damaged current
// store, whose error it would report against the wrong format — "unreadable v1
// store" for a file that is not v1 — but it is not the data-loss path it looks
// like: decodeV1 unmarshals into a map of endpoint histories, so a v2 document
// fails there too and lands in the corruption branch either way. The probe earns
// its place by naming the format the bytes actually are, not by catching a
// conversion whose absence is currently guaranteed by the field types.
func decodeStore(data []byte) (*persistedState, bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, false, fmt.Errorf("not a JSON object: %w", err)
	}

	if _, versioned := probe["version"]; !versioned {
		state, err := decodeV1(data)
		return state, true, err
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false, fmt.Errorf("unreadable store document: %w", err)
	}
	if state.Version != storeVersion {
		return nil, false, fmt.Errorf("store document is version %d and this build writes version %d", state.Version, storeVersion)
	}
	return &state, false, nil
}

// decodeV1 reads the bare map of endpoints that v1 wrote, and wraps it in a
// current document holding one project.
func decodeV1(data []byte) (*persistedState, error) {
	var histories map[string]*types.EndpointHistory
	if err := json.Unmarshal(data, &histories); err != nil {
		return nil, fmt.Errorf("unreadable v1 store: %w", err)
	}
	if histories == nil {
		histories = make(map[string]*types.EndpointHistory)
	}

	return &persistedState{
		Version: storeVersion,
		Active:  defaultProjectID,
		Projects: []types.Project{{
			ID:        defaultProjectID,
			Name:      defaultProjectName,
			CreatedAt: time.Now(),
		}},
		Histories: map[string]map[string]*types.EndpointHistory{
			defaultProjectID: histories,
		},
	}, nil
}

// keepV1CopyThenRewrite preserves the v1 document under a name of its own before
// the current format replaces it.
//
// The copy is written first and the new document second, and the order is the
// whole point. baselines.json holds the only copy of the user's contracts until
// the new document is on disk, so the sequence has to be one where a failure at
// any point leaves at least one complete copy: write the backup, verify it
// parses, then overwrite the original. Doing it the other way round — write the
// new document, then move the old one aside — does not merely risk the data, it
// cannot work at all: the write has already replaced the file, so the "old" file
// being moved aside is the new one, and the only copy of v1 is gone.
//
// Returns an error, having changed nothing, if the copy cannot be made. The next
// start reads the untouched v1 file and tries again.
//
// A conversion that works is not an error: it is announced and the function
// returns nil. Returning a notice through the error return would mean the only
// way to say "this went fine" is to say "this failed", and the next person to
// call this would reasonably treat it as one.
func (s *Store) keepV1CopyThenRewrite(v1Data []byte) error {
	backup := s.persistPath + ".v1"
	if err := atomicWriteFile(backup, v1Data, 0600); err != nil {
		return fmt.Errorf("could not keep a copy of the old store at %s, so %s has been left as it was and will be converted again next start: %w", backup, s.persistPath, err)
	}

	// Read it back and parse it, so "the copy was kept" is something this
	// function knows rather than something it assumes.
	check, err := os.ReadFile(backup)
	if err != nil {
		return fmt.Errorf("kept a copy of the old store at %s but could not read it back (%v), so %s has been left as it was", backup, err, s.persistPath)
	}
	if _, converted, err := decodeStore(check); err != nil || !converted {
		return fmt.Errorf("the copy of the old store at %s did not read back as valid (%v), so %s has been left as it was", backup, err, s.persistPath)
	}

	// writeMx is taken here rather than left to the caller because persistLocked
	// requires it: main.go's load path runs before anything else can see the
	// store, so there is no contention to lose, but relying on that would make
	// this function correct only for as long as that stays true.
	s.writeMx.Lock()
	err = s.persistLocked()
	s.writeMx.Unlock()
	if err != nil {
		return fmt.Errorf("converted the store to version %d but could not write it (%v); the old store is at %s", storeVersion, err, backup)
	}

	// Said out loud because it is a one-time change to a file the user may have
	// backed up, scripted against, or simply want to know about. The v1 version
	// of this file was silent about everything it did to it.
	log.Printf("[Driftwood] store converted to version %d; the previous file was kept at %s", storeVersion, backup)
	return nil
}
