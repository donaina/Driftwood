package types

import "testing"

// TestEveryDiffKindIsInTheList is the guard DiffKinds' own comment promises.
//
// A kind added to the constants and not to the list is invisible to every
// caller that iterates it: the dashboard renders no row for it, so it cannot be
// configured, and — worse — it silently takes the default floor, so it keeps
// alerting at a setting the operator never chose. Nothing else in the build
// compares the two.
func TestEveryDiffKindIsInTheList(t *testing.T) {
	declared := []DiffKind{
		KindTypeMismatch,
		KindRemovedField,
		KindAddedField,
		KindNullabilityChange,
		KindArrayTypeMismatch,
		KindFormatChange,
		KindStatusCodeChange,
	}

	seen := map[DiffKind]bool{}
	for _, kind := range DiffKinds {
		if seen[kind] {
			t.Errorf("DiffKinds lists %q twice", kind)
		}
		seen[kind] = true

		if !IsDiffKind(kind) {
			t.Errorf("IsDiffKind(%q) = false for a kind DiffKinds lists", kind)
		}
	}

	for _, kind := range declared {
		if !seen[kind] {
			t.Errorf("%q is a declared kind but DiffKinds does not list it, so no caller iterating the list "+
				"can see it, configure a floor for it, or render it", kind)
		}
	}

	if len(DiffKinds) != len(declared) {
		t.Errorf("DiffKinds holds %d kinds and this test names %d — one of the two moved without the other",
			len(DiffKinds), len(declared))
	}
}

// A severity nothing recognises must rank below every floor, never above them.
// Ranking it high would turn a malformed document into a page, which is the one
// failure a threshold can cause that nobody can explain afterwards.
func TestUnrecognisedSeverityRanksBelowEveryFloor(t *testing.T) {
	if got := SeverityRank(DiffSeverity("CATASTROPHIC")); got != rankUnknown {
		t.Errorf("SeverityRank(CATASTROPHIC) = %d, want %d", got, rankUnknown)
	}
	if IsSeverity(DiffSeverity("")) {
		t.Error("IsSeverity(\"\") = true; an empty severity is not one this build computes")
	}
	for _, s := range []DiffSeverity{SeverityInfo, SeverityWarning, SeverityBreaking} {
		if !IsSeverity(s) {
			t.Errorf("IsSeverity(%q) = false", s)
		}
	}

	if !(SeverityRank(SeverityBreaking) > SeverityRank(SeverityWarning) &&
		SeverityRank(SeverityWarning) > SeverityRank(SeverityInfo) &&
		SeverityRank(SeverityInfo) > rankUnknown) {
		t.Error("the severities are not ordered BREAKING > WARNING > INFO > unknown")
	}

	cfg := ThresholdConfig{}
	unknown := DiffDelta{Kind: KindTypeMismatch, Severity: DiffSeverity("CATASTROPHIC")}
	if cfg.Raises(unknown) {
		t.Error("a delta with an unrecognised severity crossed a floor")
	}
}

// The zero config is the default floor for every kind, which is what makes an
// install that has never opened the screen behave exactly as it did before the
// floor existed.
func TestZeroConfigIsTheDefaultFloor(t *testing.T) {
	var cfg ThresholdConfig

	for _, kind := range DiffKinds {
		if got := cfg.FloorFor(kind); got != DefaultFloor {
			t.Errorf("FloorFor(%s) on the zero config = %q, want %q", kind, got, DefaultFloor)
		}
	}
	if got := cfg.Preset(); got != PresetRecommended {
		t.Errorf("the zero config reports preset %q, want %q", got, PresetRecommended)
	}

	// The default has to be WARNING or the predicate silently changes meaning:
	// the store raised an alert for HasBreakingChanges || HasWarnings, which is
	// "some delta reached WARNING or higher".
	if DefaultFloor != SeverityWarning {
		t.Errorf("DefaultFloor = %q, want WARNING — any other value changes which alerts a "+
			"pre-existing install raises", DefaultFloor)
	}
}

// Crosses reads the deltas, not the flags.
//
// This is the behaviour change the floor forced: the two booleans on
// ContractDiff have already collapsed seven kinds into two, so a per-kind floor
// cannot be answered from them. The store's producers all derive the flags from
// the deltas, so the two agree in production — and this pins that when they do
// not, the deltas are the ones that count.
func TestCrossesReadsDeltasNotFlags(t *testing.T) {
	var cfg ThresholdConfig

	infoOnly := &ContractDiff{
		HasBreakingChanges: true,
		HasWarnings:        true,
		Deltas: []DiffDelta{
			{JSONPath: "$.extra", Kind: KindAddedField, Severity: SeverityInfo},
		},
	}
	if cfg.Crosses(infoOnly) {
		t.Error("an INFO delta crossed the default floor on the strength of the flags alone")
	}

	if cfg.Crosses(nil) {
		t.Error("a nil diff crossed a floor")
	}
	if cfg.Crosses(&ContractDiff{HasBreakingChanges: true}) {
		t.Error("a diff with no deltas crossed a floor; it names no change to be alerted about")
	}

	breaking := &ContractDiff{
		Deltas: []DiffDelta{
			{JSONPath: "$.id", Kind: KindRemovedField, Severity: SeverityBreaking},
		},
	}
	if !cfg.Crosses(breaking) {
		t.Error("a BREAKING delta did not cross the default floor")
	}
}

// A floor is per kind: lowering one kind must not lower any other. Without this
// the config would be a single global severity wearing a table.
func TestFloorsArePerKind(t *testing.T) {
	cfg := ThresholdConfig{Floors: map[DiffKind]DiffSeverity{
		KindAddedField: SeverityInfo,
	}}

	added := DiffDelta{Kind: KindAddedField, Severity: SeverityInfo}
	if !cfg.Raises(added) {
		t.Error("the lowered kind did not raise at INFO")
	}

	// Same severity, a kind the config says nothing about: still the default.
	removed := DiffDelta{Kind: KindRemovedField, Severity: SeverityInfo}
	if cfg.Raises(removed) {
		t.Error("a kind the config does not mention was lowered along with one it does")
	}

	if got := cfg.Preset(); got != PresetCustom {
		t.Errorf("a config with one kind lowered reports preset %q, want %q", got, PresetCustom)
	}
}

// The preset name is a reading of the floors, so it cannot disagree with them.
func TestPresetNamesTheFloorsItDescribes(t *testing.T) {
	for _, name := range PresetNames {
		set, ok := FloorsForPreset(name)
		if !ok {
			t.Fatalf("PresetNames lists %q but FloorsForPreset does not know it", name)
		}
		if len(set) != len(DiffKinds) {
			t.Errorf("preset %q floors %d kinds, want %d", name, len(set), len(DiffKinds))
		}

		cfg := ThresholdConfig{Floors: set}
		if got := cfg.Preset(); got != name {
			t.Errorf("a config built from preset %q reports %q", name, got)
		}
		for _, kind := range DiffKinds {
			if got := cfg.FloorFor(kind); got != set[kind] {
				t.Errorf("preset %q: FloorFor(%s) = %q, want %q", name, kind, got, set[kind])
			}
		}
	}

	if _, ok := FloorsForPreset(PresetCustom); ok {
		t.Error("FloorsForPreset accepts \"custom\"; it is a finding, not a choice, and expanding it " +
			"would give the dashboard a preset to apply that names no floors")
	}
	if _, ok := FloorsForPreset("paranoid"); ok {
		t.Error("FloorsForPreset accepts a name it does not define")
	}
}

func TestValidateFloorsRefusesWhatNothingCanAct(t *testing.T) {
	if err := ValidateFloors(nil); err != nil {
		t.Errorf("an empty floor map was refused: %v", err)
	}
	if err := ValidateFloors(map[DiffKind]DiffSeverity{DiffKind("HEADER_CHANGE"): SeverityInfo}); err == nil {
		t.Error("a floor for a kind the engine never emits was accepted; nothing reads it, so it is a " +
			"setting that saves successfully and changes nothing")
	}
	if err := ValidateFloors(map[DiffKind]DiffSeverity{KindAddedField: DiffSeverity("LOUD")}); err == nil {
		t.Error("a floor naming a severity this build cannot compute was accepted")
	}
	if err := ValidateFloors(map[DiffKind]DiffSeverity{KindAddedField: SeverityInfo}); err != nil {
		t.Errorf("a valid floor was refused: %v", err)
	}
}
