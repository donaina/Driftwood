package diff

import (
	"strings"
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

// A date that became a UUID keeps its JSON type, so before Format was read this
// was a MATCH. It is the clearest case of a contract change the detector could
// not see.
func TestFormatChange_TwoKnownFormats_IsBreaking(t *testing.T) {
	d, err := CompareJSON(`{"id": "2024-01-31"}`, `{"id": "550e8400-e29b-41d4-a716-446655440000"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.id")
	if delta.Kind != types.KindFormatChange {
		t.Errorf("kind = %s, want FORMAT_CHANGE", delta.Kind)
	}
	if delta.Severity != types.SeverityBreaking {
		t.Errorf("severity = %s, want BREAKING", delta.Severity)
	}
	if delta.Expected != "date" || delta.Actual != "uuid" {
		t.Errorf("expected/actual = %s/%s, want date/uuid", delta.Expected, delta.Actual)
	}
	if !d.HasBreakingChanges {
		t.Error("HasBreakingChanges = false: the change would not raise an alert")
	}
}

// A timestamp losing its time component is the same class of change: clients
// parsing RFC3339 stop parsing.
func TestFormatChange_DateTimeToDate_IsBreaking(t *testing.T) {
	d, err := CompareJSON(`{"published_on": "2024-01-31T10:00:00Z"}`, `{"published_on": "2024-01-31"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	if got := deltaAt(t, d, "$.published_on").Severity; got != types.SeverityBreaking {
		t.Errorf("severity = %s, want BREAKING", got)
	}
}

// Weaker evidence: the value is no longer date-shaped, but it may be a
// placeholder rather than a different kind of value. Reported at the warning
// tier, which records it without raising an alert.
func TestFormatChange_FormatNoLongerMatches_IsWarning(t *testing.T) {
	d, err := CompareJSON(`{"starts_on": "2024-01-31"}`, `{"starts_on": "N/A"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.starts_on")
	if delta.Severity != types.SeverityWarning {
		t.Errorf("severity = %s, want WARNING", delta.Severity)
	}
	if delta.Actual != "<absent>" {
		t.Errorf("actual = %q, want the canonical <absent> sentinel", delta.Actual)
	}
	if d.HasBreakingChanges {
		t.Error("HasBreakingChanges = true: a value that stopped looking like a date raised an alert")
	}
}

func TestFormatChange_FormatGained_IsInfo(t *testing.T) {
	d, err := CompareJSON(`{"id": "sample"}`, `{"id": "550e8400-e29b-41d4-a716-446655440000"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.id")
	if delta.Severity != types.SeverityInfo {
		t.Errorf("severity = %s, want INFO", delta.Severity)
	}
	if delta.Expected != "<absent>" {
		t.Errorf("expected = %q, want <absent>", delta.Expected)
	}
	if d.HasBreakingChanges || d.HasWarnings {
		t.Error("gaining a format was reported above the info tier")
	}
}

// The common case, and the one that must stay silent: an unchanged field. This
// is the demo payload's email against itself.
func TestFormatUnchanged_ProducesNoDelta(t *testing.T) {
	payload := `{"id": 1, "email": "alex@company.com", "joined": "2024-01-31", "token": "550e8400-e29b-41d4-a716-446655440000"}`

	d, err := CompareJSON(payload, payload)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(d.Deltas) != 0 {
		t.Errorf("an identical payload produced %d deltas: %+v", len(d.Deltas), d.Deltas)
	}
}

// A stored schema's format must be honoured the same way its required list is:
// the spec said date-time, the response is a date, and re-inferring the baseline
// sample is not what decides it.
func TestFormatChange_AgainstAStoredSchema(t *testing.T) {
	baseline := node(t, `{"published_on": "2024-01-31T10:00:00Z"}`)
	if baseline.Properties["published_on"].Format != "date-time" {
		t.Fatalf("test premise: inferred format = %q, want date-time", baseline.Properties["published_on"].Format)
	}

	d, err := CompareJSONWithSchema(baseline, `{"published_on": "2024-01-31"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.published_on")
	if delta.Kind != types.KindFormatChange || delta.Severity != types.SeverityBreaking {
		t.Errorf("kind/severity = %s/%s, want FORMAT_CHANGE/BREAKING", delta.Kind, delta.Severity)
	}
}

// Nested and array-element fields must be reached by the same check, since the
// recursion is what carries it there.
func TestFormatChange_IsReachedThroughNestingAndArrays(t *testing.T) {
	baseline := `{"user": {"seen": "2024-01-31"}, "events": [{"at": "2024-01-31T10:00:00Z"}]}`
	current := `{"user": {"seen": "550e8400-e29b-41d4-a716-446655440000"}, "events": [{"at": "2024-01-31"}]}`

	d, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	for _, path := range []string{"$.user.seen", "$.events[*].at"} {
		if got := deltaAt(t, d, path).Kind; got != types.KindFormatChange {
			t.Errorf("%s kind = %s, want FORMAT_CHANGE", path, got)
		}
	}
}

// The message convention: structural, lowercase, no prose sentence.
func TestFormatChange_MessagesStayStructural(t *testing.T) {
	payloads := [][2]string{
		{`{"v": "2024-01-31"}`, `{"v": "550e8400-e29b-41d4-a716-446655440000"}`},
		{`{"v": "2024-01-31"}`, `{"v": "N/A"}`},
		{`{"v": "plain"}`, `{"v": "2024-01-31"}`},
	}

	for _, p := range payloads {
		d, err := CompareJSON(p[0], p[1])
		if err != nil {
			t.Fatalf("compare: %v", err)
		}
		if len(d.Deltas) != 1 {
			t.Fatalf("want 1 delta, got %+v", d.Deltas)
		}
		msg := d.Deltas[0].Message
		if containsEmoji(msg) || containsProseWords(msg) {
			t.Errorf("message %q breaks the structural convention", msg)
		}
		if msg != strings.ToLower(msg) {
			t.Errorf("message %q is not lowercase", msg)
		}
	}
}

// An empty array's item schema is unknown, and an unknown node is compatible
// with every type, so this reaches the format check. It must not be reported as
// a format change: the value is absent, not a string in a different shape.
// Without the check that the current node is also a string, every endpoint
// returning an empty list where it used to return dates reports a spurious
// warning.
func TestFormatChange_EmptyArrayIsNotAFormatChange(t *testing.T) {
	d, err := CompareJSON(`{"tags": ["2024-01-31"]}`, `{"tags": []}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	for _, delta := range d.Deltas {
		if delta.Kind == types.KindFormatChange {
			t.Errorf("%s reported %s=%s, but the current value is absent rather than a differently-shaped string",
				delta.JSONPath, delta.Kind, delta.Severity)
		}
	}
}
