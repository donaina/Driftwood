package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

// diffTraffic builds one request whose contract comparison produced these
// deltas, which is all AddTraffic reads to decide whether an alert exists.
func diffTraffic(id string, deltas ...types.DiffDelta) types.CapturedTraffic {
	return types.CapturedTraffic{
		ID:             id,
		Method:         "GET",
		Path:           "/api/users",
		StatusCode:     200,
		ContractStatus: "MATCH",
		Diff:           &types.ContractDiff{Deltas: deltas},
	}
}

func infoDelta() types.DiffDelta {
	return types.DiffDelta{JSONPath: "$.extra", Kind: types.KindAddedField, Severity: types.SeverityInfo}
}

func warningDelta() types.DiffDelta {
	return types.DiffDelta{JSONPath: "$.nickname", Kind: types.KindRemovedField, Severity: types.SeverityWarning}
}

func breakingDelta() types.DiffDelta {
	return types.DiffDelta{JSONPath: "$.id", Kind: types.KindRemovedField, Severity: types.SeverityBreaking}
}

func preset(t *testing.T, name string) map[types.DiffKind]types.DiffSeverity {
	t.Helper()
	floors, ok := types.FloorsForPreset(name)
	if !ok {
		t.Fatalf("FloorsForPreset(%q) is not defined", name)
	}
	return floors
}

// TestTheAlertPredicateIsTheStoredFloor is the test this phase exists for.
//
// Before it, the store raised an alert on the literal HasBreakingChanges ||
// HasWarnings and the Thresholds screen configured five severities that nothing
// read. The screen said one thing and the engine did another. Both directions
// are asserted here, because a predicate that only ever got *wider* would pass a
// test that only lowered the floor.
func TestTheAlertPredicateIsTheStoredFloor(t *testing.T) {
	s := testStore(t)
	project := s.ActiveProject()

	// The default floor, which is what every install that never opens the screen
	// has. An added field is recorded and does not alert.
	if alert := s.AddTraffic(project, diffTraffic("tr_info_default", infoDelta())); alert != nil {
		t.Error("an INFO delta raised an alert at the default floor; the default is meant to be exactly " +
			"BREAKING || WARNING, which an added field is not")
	}
	// A warning does, so the default is not silently "nothing alerts".
	if alert := s.AddTraffic(project, diffTraffic("tr_warning_default", warningDelta())); alert == nil {
		t.Error("a WARNING delta raised no alert at the default floor")
	}
	if alert := s.AddTraffic(project, diffTraffic("tr_breaking_default", breakingDelta())); alert == nil {
		t.Error("a BREAKING delta raised no alert at the default floor")
	}
	if got := len(s.GetAlerts(project, 10)); got != 2 {
		t.Fatalf("alerts at the default floor = %d, want 2 (the warning and the break, not the addition)", got)
	}

	// Lowered: informational changes become alerts.
	if _, err := s.SetThresholds(project, preset(t, types.PresetStrict)); err != nil {
		t.Fatalf("SetThresholds(strict): %v", err)
	}
	if alert := s.AddTraffic(project, diffTraffic("tr_info_strict", infoDelta())); alert == nil {
		t.Error("an INFO delta raised no alert under a floor of INFO, so the configured floor is not the " +
			"predicate — which is the whole feature")
	}
	if got := len(s.GetAlerts(project, 10)); got != 3 {
		t.Errorf("alerts under a floor of INFO = %d, want 3", got)
	}

	// Raised the other way: warnings stop alerting.
	if _, err := s.SetThresholds(project, preset(t, types.PresetLenient)); err != nil {
		t.Fatalf("SetThresholds(lenient): %v", err)
	}
	if alert := s.AddTraffic(project, diffTraffic("tr_warning_lenient", warningDelta())); alert != nil {
		t.Error("a WARNING delta raised an alert under a floor of BREAKING")
	}
	if alert := s.AddTraffic(project, diffTraffic("tr_breaking_lenient", breakingDelta())); alert == nil {
		t.Error("a BREAKING delta raised no alert under a floor of BREAKING")
	}
	if got := len(s.GetAlerts(project, 10)); got != 4 {
		t.Errorf("alerts under a floor of BREAKING = %d, want 4", got)
	}
}

// A floor is one project's. A single field on the store would pass every test
// above and hand one client's sensitivity to another.
func TestThresholdsAreScopedToOneProject(t *testing.T) {
	s := testStore(t)
	other := secondProject(t, s, "Other")

	if _, err := s.SetThresholds(other, preset(t, types.PresetLenient)); err != nil {
		t.Fatalf("SetThresholds: %v", err)
	}

	// The project that was not configured keeps the default.
	if alert := s.AddTraffic(s.ActiveProject(), diffTraffic("tr_default_warning", warningDelta())); alert == nil {
		t.Error("configuring one project's floor changed another's: a warning stopped alerting on the " +
			"project that was never configured")
	}
	if alert := s.AddTraffic(other, diffTraffic("tr_other_warning", warningDelta())); alert != nil {
		t.Error("the configured project did not use its own floor")
	}

	if got := s.GetThresholds(other).Preset(); got != types.PresetLenient {
		t.Errorf("the configured project reports preset %q, want %q", got, types.PresetLenient)
	}
	if got := s.GetThresholds(s.ActiveProject()).Preset(); got != types.PresetRecommended {
		t.Errorf("the unconfigured project reports preset %q, want %q", got, types.PresetRecommended)
	}
}

// A project nobody has configured answers with the default, and that is not a
// placeholder: the default is what AddTraffic applies.
func TestAnUnconfiguredProjectReadsAsTheDefaultFloor(t *testing.T) {
	s := testStore(t)

	got := s.GetThresholds(s.ActiveProject())
	if got.Preset() != types.PresetRecommended {
		t.Errorf("preset = %q, want %q", got.Preset(), types.PresetRecommended)
	}
	for _, kind := range types.DiffKinds {
		if floor := got.FloorFor(kind); floor != types.DefaultFloor {
			t.Errorf("FloorFor(%s) = %q, want %q", kind, floor, types.DefaultFloor)
		}
	}
	if !got.UpdatedAt.IsZero() {
		t.Error("a project that has never saved a floor reported a save time")
	}
}

func TestSetThresholdsRefusesAProjectThatDoesNotExist(t *testing.T) {
	s := testStore(t)

	if _, err := s.SetThresholds("no-such-project", preset(t, types.PresetStrict)); !errors.Is(err, ErrNoSuchProject) {
		t.Errorf("SetThresholds on a missing project = %v, want ErrNoSuchProject", err)
	}
	// And it must not have conjured one, the way ensureProjectLocked deliberately
	// refuses to for reads.
	if s.ProjectExists("no-such-project") {
		t.Error("refusing the write still created the project")
	}
}

func TestSetThresholdsRefusesAFloorNothingCanActOn(t *testing.T) {
	s := testStore(t)

	if _, err := s.SetThresholds(s.ActiveProject(), map[types.DiffKind]types.DiffSeverity{
		"HEADER_CHANGE": types.SeverityInfo,
	}); err == nil {
		t.Error("a floor for a kind the engine never emits was stored; nothing reads it, so the screen " +
			"would report a save that changed nothing")
	}
	if _, err := s.SetThresholds(s.ActiveProject(), map[types.DiffKind]types.DiffSeverity{
		types.KindAddedField: "LOUD",
	}); err == nil {
		t.Error("a floor naming a severity this build cannot compute was stored")
	}
}

func TestThresholdsSurviveAReload(t *testing.T) {
	s := testStore(t)
	project := s.ActiveProject()

	// Lenient, because the assertion below is that a warning stops alerting, and
	// only a floor above WARNING can make that true. Saving the default floor
	// would leave the check passing for the wrong reason.
	floors := preset(t, types.PresetLenient)
	if _, err := s.SetThresholds(project, floors); err != nil {
		t.Fatalf("SetThresholds: %v", err)
	}

	reloaded := reopenStore(t, s)
	if err := reloaded.loadFromFile(); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}

	got := reloaded.GetThresholds(project)
	if got.Preset() != types.PresetLenient {
		t.Errorf("preset after reload = %q, want %q", got.Preset(), types.PresetLenient)
	}
	for _, kind := range types.DiffKinds {
		if floor := got.FloorFor(kind); floor != floors[kind] {
			t.Errorf("FloorFor(%s) after reload = %q, want %q", kind, floor, floors[kind])
		}
	}
	// The proof that it is the stored floor and not a coincidence: a warning no
	// longer alerts on the reloaded store.
	if alert := reloaded.AddTraffic(project, diffTraffic("tr_after_reload", warningDelta())); alert != nil {
		t.Error("the reloaded store alerted on a warning, so it read back the default rather than the " +
			"saved floor")
	}
}

// An unchanged document must write identical bytes. Thresholds are a map, and
// the document is written from it.
func TestSavingThresholdsKeepsAnUnchangedDocumentByteIdentical(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	s, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.SetThresholds(s.ActiveProject(), preset(t, types.PresetStrict)); err != nil {
		t.Fatalf("SetThresholds: %v", err)
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}

	reopened, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	// A save that touches nothing. DeleteWebhook removes a kind that was never
	// configured, which is deliberately not an error, and it persists.
	if err := reopened.DeleteWebhook(reopened.ActiveProject(), "not-a-kind"); err != nil {
		t.Fatalf("triggering a save: %v", err)
	}

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store back: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("an unchanged save rewrote the document.\n--- before ---\n%s\n--- after ---\n%s", first, second)
	}
}

// The document this build writes is a version it can read, and the floors are
// under their own key rather than folded into a project.
func TestThresholdsAreWrittenAsAVersionTheBuildReads(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	s, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.SetThresholds(s.ActiveProject(), preset(t, types.PresetLenient)); err != nil {
		t.Fatalf("SetThresholds: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	var doc persistedState
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("the written document does not decode: %v", err)
	}
	if doc.Version != storeVersion {
		t.Errorf("written version = %d, want %d", doc.Version, storeVersion)
	}
	if doc.Version <= 3 {
		t.Error("thresholds were written at a version a build without them would also claim to read; " +
			"that build would drop the key on its next save")
	}
	if _, ok := doc.Thresholds[s.ActiveProject()]; !ok {
		t.Errorf("the document has no thresholds entry for %q", s.ActiveProject())
	}
}

// A document written before thresholds existed loads, keeps its endpoints, and
// still alerts the way it used to — without being rewritten merely for being
// opened.
func TestV3StoreLoadsAndKeepsItsAlertBehaviour(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	// Built from the real type, the way TestV2StoreLoadsWithNoWebhooks builds
	// its fixture: version 3 with Thresholds omitted by omitempty is exactly what
	// a v3 build wrote.
	doc := persistedState{
		Version:  3,
		Active:   defaultProjectID,
		Projects: []types.Project{{ID: defaultProjectID, Name: defaultProjectName}},
		Histories: map[string]map[string]*types.EndpointHistory{
			defaultProjectID: {},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("building the v3 fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing the v3 fixture: %v", err)
	}

	s, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("a v3 store was refused: %v", err)
	}

	// Reading it must not have rewritten it. The version is stamped on the next
	// natural save, not on load.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store back: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Error("loading a v3 store rewrote it")
	}

	if got := s.GetThresholds(defaultProjectID).Preset(); got != types.PresetRecommended {
		t.Errorf("a v3 store reports preset %q, want the default %q", got, types.PresetRecommended)
	}
	if alert := s.AddTraffic(defaultProjectID, diffTraffic("tr_v3_warning", warningDelta())); alert == nil {
		t.Error("a v3 store stopped alerting on warnings, which is the behaviour it had when it was written")
	}
	if alert := s.AddTraffic(defaultProjectID, diffTraffic("tr_v3_info", infoDelta())); alert != nil {
		t.Error("a v3 store started alerting on informational changes, which it never did before")
	}
}
