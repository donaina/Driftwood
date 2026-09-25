package types

import (
	"fmt"
	"time"
)

// DiffKinds is every delta kind, in a fixed order.
//
// A slice rather than a map for the reason WebhookKinds is one: the store needs
// a deterministic order so an unchanged document writes byte-identical bytes,
// and the dashboard needs one so its rows do not reorder themselves between
// loads.
//
// The compiler cannot check that this list and the Kind* constants agree — a
// kind added to one and not the other is a delta nothing can configure a floor
// for, which reads as a delta whose floor is the default. TestEveryDiffKindIsIn
// TheList is what closes that gap.
var DiffKinds = []DiffKind{
	KindTypeMismatch,
	KindRemovedField,
	KindAddedField,
	KindNullabilityChange,
	KindArrayTypeMismatch,
	KindFormatChange,
	KindStatusCodeChange,
}

// IsDiffKind reports whether kind names a difference this build can report.
func IsDiffKind(kind DiffKind) bool {
	for _, k := range DiffKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// The severities ranked, so a floor can be compared against what a delta
// measured. Ordinary constants rather than a map because this is the ordering,
// not data: a map would make it something a caller could edit.
const (
	rankUnknown  = 0
	rankInfo     = 1
	rankWarning  = 2
	rankBreaking = 3
)

// SeverityRank orders severities from least to most severe.
//
// An unrecognised severity ranks below every floor rather than above them. A
// delta whose severity nothing can name is not a reason to wake somebody up,
// and it is also what the predicate did before floors existed: the flags it read
// were set only by a delta that was exactly BREAKING or exactly WARNING, so an
// unnamed severity raised nothing then either. Ranking it 0 keeps that, and
// keeps it from panicking on a hand-written document.
func SeverityRank(s DiffSeverity) int {
	switch s {
	case SeverityInfo:
		return rankInfo
	case SeverityWarning:
		return rankWarning
	case SeverityBreaking:
		return rankBreaking
	default:
		return rankUnknown
	}
}

// IsSeverity reports whether s names a severity this build computes.
func IsSeverity(s DiffSeverity) bool {
	return SeverityRank(s) > rankUnknown
}

// DefaultFloor is the severity a kind has to reach to raise an alert when a
// project has expressed no preference.
//
// WARNING, because that is what this product did before the floor existed: the
// store raised an alert for HasBreakingChanges || HasWarnings, which is exactly
// "some delta reached WARNING or higher". An install that never opens this
// screen therefore keeps the behaviour it had, byte for byte, which is what
// makes a configurable floor additive rather than a migration.
const DefaultFloor = SeverityWarning

// ThresholdConfig is one project's alert floor: the least severe delta of each
// kind that raises an alert.
//
// A floor decides what becomes an alert. It never changes what severity a delta
// has, and that is the load-bearing half of the design. Severity is a
// measurement — BREAKING means a promise the baseline made was broken — so a
// config able to rewrite it would let the dashboard show MATCH beside a
// BREAKING delta, and would make the README's severity table describe something
// other than what the engine does.
//
// The zero value is the default behaviour, and that is deliberate rather than
// incidental: a project that has never opened this screen holds no
// ThresholdConfig at all, so "unconfigured" and ThresholdConfig{} have to mean
// the same thing. FloorFor is what makes them.
type ThresholdConfig struct {
	Floors    map[DiffKind]DiffSeverity `json:"floors,omitempty"`
	UpdatedAt time.Time                 `json:"updated_at,omitempty"`
}

// FloorFor returns the floor in effect for one kind: what the config says, or
// the default when the config does not mention it.
//
// Absent means default, not "never alerts". A config written by a build that
// knew fewer kinds cannot leave a kind unmonitored by omission, which is the
// failure that would go unnoticed longest — alerts simply stop, and nothing on
// screen says why.
func (c ThresholdConfig) FloorFor(kind DiffKind) DiffSeverity {
	if floor, ok := c.Floors[kind]; ok {
		return floor
	}
	return DefaultFloor
}

// Raises reports whether a delta crosses the floor: whether something this
// severe, of this kind, becomes an alert.
func (c ThresholdConfig) Raises(d DiffDelta) bool {
	return SeverityRank(d.Severity) >= SeverityRank(c.FloorFor(d.Kind))
}

// Crosses reports whether any delta in a diff crosses the floor — the question
// the store asks to decide whether a request raised an alert.
//
// A diff with no deltas raises nothing, whatever its flags say. Every producer
// of a ContractDiff in this repository derives those flags from its deltas
// (CompareSchemas) or sets them beside one (the proxy's 5xx and 204 paths), so
// the two cannot disagree in production — and a diff that claims a break while
// naming none is a value this product does not produce. Reading the deltas
// rather than the flags is what lets a per-kind floor exist at all: the flags
// have already collapsed seven kinds into two booleans.
func (c ThresholdConfig) Crosses(d *ContractDiff) bool {
	if d == nil {
		return false
	}
	for _, delta := range d.Deltas {
		if c.Raises(delta) {
			return true
		}
	}
	return false
}

// The presets, each a named floor applied to every kind.
//
// They are the existing dashboard's three names, kept because the words are
// right, carrying meanings the engine can honour. What the old component
// meant by them was not: it read "Strict" as "everything is BREAKING", which
// asks a config to overwrite a measurement. A floor can only say how much
// reaches you, so Strict is the lowest floor, not the loudest label.
const (
	// PresetStrict alerts on every difference, informational ones included.
	PresetStrict = "strict"
	// PresetRecommended alerts on breaking changes and warnings. This is
	// DefaultFloor applied uniformly, so it is what an unconfigured project is.
	PresetRecommended = "recommended"
	// PresetLenient alerts only on breaking changes.
	PresetLenient = "lenient"
	// PresetCustom is not selectable. It is what Preset reports when a project's
	// floors are not one preset applied uniformly.
	PresetCustom = "custom"
)

// PresetNames is every preset a project can choose, in the order the dashboard
// offers them. PresetCustom is absent because it is a finding, not a choice.
var PresetNames = []string{PresetStrict, PresetRecommended, PresetLenient}

// presetFloor is the floor each preset puts under every kind.
var presetFloor = map[string]DiffSeverity{
	PresetStrict:      SeverityInfo,
	PresetRecommended: SeverityWarning,
	PresetLenient:     SeverityBreaking,
}

// FloorsForPreset returns the floors a preset names, for a caller building a
// config from a preset rather than from a form.
func FloorsForPreset(name string) (map[DiffKind]DiffSeverity, bool) {
	floor, ok := presetFloor[name]
	if !ok {
		return nil, false
	}
	floors := make(map[DiffKind]DiffSeverity, len(DiffKinds))
	for _, kind := range DiffKinds {
		floors[kind] = floor
	}
	return floors, true
}

// Preset returns the preset these floors amount to, or PresetCustom when they
// are not one preset applied uniformly.
//
// Derived rather than stored, which is the whole point. A stored name beside
// the floors it describes is two answers to one question, and nothing would
// keep them agreeing: the dashboard could write "strict" over floors that are
// not strict, and every reader that trusted the name would be wrong about what
// raises an alert. Here the name cannot disagree with the floors, because it is
// a reading of them.
func (c ThresholdConfig) Preset() string {
	name := ""
	for _, kind := range DiffKinds {
		floor := c.FloorFor(kind)

		match := ""
		for _, candidate := range PresetNames {
			if presetFloor[candidate] == floor {
				match = candidate
				break
			}
		}
		if match == "" {
			return PresetCustom
		}
		if name == "" {
			name = match
			continue
		}
		if name != match {
			return PresetCustom
		}
	}

	// Only reachable if DiffKinds is empty, which would make every config
	// vacuously uniform. Reporting that as a preset would be a name for nothing.
	if name == "" {
		return PresetCustom
	}
	return name
}

// ValidateFloors reports whether every entry names a kind and a severity this
// build can act on.
//
// Validated at the boundary rather than in the store, and refused rather than
// stored, for the reason IsWebhookKind gives: a floor for a kind nothing emits
// is a setting with no effect, and a setting with no effect is the shape of lie
// this whole feature exists to remove. The dashboard renders every kind, so a
// body it produced always passes.
func ValidateFloors(floors map[DiffKind]DiffSeverity) error {
	for kind, floor := range floors {
		if !IsDiffKind(kind) {
			return fmt.Errorf("unknown delta kind %q", kind)
		}
		if !IsSeverity(floor) {
			return fmt.Errorf("unknown severity %q for %s", floor, kind)
		}
	}
	return nil
}
