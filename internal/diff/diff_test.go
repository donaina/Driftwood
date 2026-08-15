package diff

import (
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

func TestTypeMismatchDetection(t *testing.T) {
	baseline := `{"id": 1024, "name": "Alice", "is_active": true}`
	current := `{"id": "1024", "name": "Alice", "is_active": true}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !res.HasBreakingChanges {
		t.Errorf("expected breaking changes, got none")
	}

	if len(res.Deltas) == 0 {
		t.Fatalf("expected at least 1 delta")
	}

	delta := res.Deltas[0]
	if delta.JSONPath != "$.id" {
		t.Errorf("expected path $.id, got %s", delta.JSONPath)
	}
	if delta.Kind != types.KindTypeMismatch {
		t.Errorf("expected TYPE_MISMATCH, got %s", delta.Kind)
	}
}

func TestRemovedFieldDetection(t *testing.T) {
	baseline := `{"id": 1024, "email": "alice@example.com", "role": "admin"}`
	current := `{"id": 1024, "role": "admin"}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !res.HasBreakingChanges {
		t.Errorf("expected breaking changes for missing email field")
	}

	found := false
	for _, d := range res.Deltas {
		if d.JSONPath == "$.email" && d.Kind == types.KindRemovedField {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected missing email delta")
	}
}

// --- PR #2 reproducer tests (should fail before fixes) ---

// #1: isCompatibleType should accept integer↔number in BOTH directions
func TestIntegerNumberCompatibility_BothDirections(t *testing.T) {
	// baseline integer, current number (currently works)
	baseline := `{"count": 42}`
	current := `{"count": 42.0}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasBreakingChanges {
		t.Errorf("integer→number should be compatible, got breaking")
	}

	// baseline number, current integer (BUG: currently BREAKING, should be compatible)
	baseline = `{"count": 42.0}`
	current = `{"count": 42}`

	res, err = CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasBreakingChanges {
		t.Errorf("number→integer should be compatible (bug #1), got breaking")
	}
}

// #2: nullability fall-through - should continue checking after nullable=true
func TestNullabilityFallthrough(t *testing.T) {
	// Baseline: non-nullable integer, nullable field (so null is ok), but ALSO current is string (type mismatch)
	// This should still detect the type mismatch even though nullable=true
	baseline := `{"id": 42}`
	current := `{"id": "not-an-int"}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasBreakingChanges {
		t.Errorf("expected breaking for type mismatch (string vs int), even with nullable fallthrough (bug #2)")
	}

	// Now test nullable field where current is null - should be INFO (or not breaking)
	baseline = `{"id": 42}`
	current = `{"id": null}`

	// Current behavior: returns early at line 87 after nullability check, never reaches type check
	// Fixed behavior: should check type compatibility after nullable check
}

// #3: baseline null → real type should be INFO (contract refinement), not BREAKING
func TestNullToTyped_IsInfoNotBreaking(t *testing.T) {
	baseline := `{"id": null}`
	current := `{"id": 42}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Deltas) == 0 {
		t.Fatalf("expected a delta for null→typed")
	}

	delta := res.Deltas[0]
	if delta.Severity != types.SeverityInfo {
		t.Errorf("null→typed should be INFO (bug #3), got %s", delta.Severity)
	}
	if delta.Kind != types.KindNullabilityChange && delta.Kind != types.KindTypeMismatch {
		t.Errorf("expected NULLABILITY_CHANGE or TYPE_MISMATCH, got %s", delta.Kind)
	}
}

// #6: empty-array unknown item should NOT be silently compatible
func TestEmptyArrayUnknownItem_NotSilentlyCompatible(t *testing.T) {
	// Baseline: empty array (item schema = unknown)
	// Current: array with items - should NOT be compatible (should detect type mismatch)
	baseline := `{"items": []}`
	current := `{"items": [1, 2, 3]}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	// Currently: unknown matches everything (line 159-160 in diff.go)
	// Fixed: unknown from empty array should NOT match concrete types
	if !res.HasBreakingChanges {
		t.Errorf("empty array unknown item should not match concrete array (bug #6)")
	}

	found := false
	for _, d := range res.Deltas {
		if d.JSONPath == "$.items[*]" && (d.Kind == types.KindTypeMismatch || d.Kind == types.KindArrayTypeMismatch) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ARRAY_ITEM_MISMATCH/TYPE_MISMATCH for empty→concrete array")
	}
}

// #24: deterministic delta ordering - iterate sorted keys
func TestDeterministicDeltaOrdering(t *testing.T) {
	// Object with keys in different orders should produce same delta order
	baseline := `{"z": 1, "a": 2, "m": 3}`
	current := `{"z": 1, "a": "changed", "m": 3}`

	res1, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	// Same diff, swap input order in code logic
	baseline = `{"a": 2, "m": 3, "z": 1}`
	current = `{"a": "changed", "m": 3, "z": 1}`

	res2, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	// Compare delta order - should be identical
	if len(res1.Deltas) != len(res2.Deltas) {
		t.Fatalf("delta count mismatch: %d vs %d", len(res1.Deltas), len(res2.Deltas))
	}
	for i := range res1.Deltas {
		if res1.Deltas[i].JSONPath != res2.Deltas[i].JSONPath {
			t.Errorf("delta order not deterministic at index %d: %s vs %s (bug #24)",
				i, res1.Deltas[i].JSONPath, res2.Deltas[i].JSONPath)
		}
	}
}

// #25: bracket-quote non-identifier JSONPath keys
func TestJSONPathBracketQuoting(t *testing.T) {
	baseline := `{"normal_key": 1, "key with spaces": 2, "123numeric": 3}`
	current := `{"normal_key": 1, "key with spaces": "changed", "123numeric": 3}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	// Should produce bracketed paths for non-identifiers
	validIdentifier := true
	for _, d := range res.Deltas {
		// Check that key with spaces becomes $.['key with spaces'] not $.key with spaces
		if d.JSONPath == "$.key with spaces" {
			validIdentifier = false
			t.Errorf("key with spaces should be bracket-quoted, got %s (bug #25)", d.JSONPath)
		}
		if d.JSONPath == "$.123numeric" {
			validIdentifier = false
			t.Errorf("numeric key should be bracket-quoted, got %s (bug #25)", d.JSONPath)
		}
		// Valid identifier should remain simple
		if d.JSONPath == "$.normal_key" {
			// OK
		}
	}
	if !validIdentifier {
		t.Errorf("invalid identifier keys should be bracket-quoted")
	}
}

// #27: WARNING tier for type-widening, null→typed, added-required
func TestWarningTier(t *testing.T) {
	// Type widening (integer→number) should be WARNING, not BREAKING
	baseline := `{"count": 42}`
	current := `{"count": 42.0}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Deltas) > 0 && res.Deltas[0].Severity == types.SeverityBreaking {
		t.Errorf("type widening integer→number should be WARNING, not BREAKING (bug #27)")
	}

	// null→typed should be INFO (already tested in TestNullToTyped_IsInfoNotBreaking)

	// Added required field (if we had freq-based) - not testing yet as it's PR #5
}

// #29: structural deltas only - no emoji/prose in Message, one canonical "absent" sentinel
func TestStructuralDeltas_NoEmojiProse(t *testing.T) {
	baseline := `{"id": 1024}`
	current := `{"id": "1024"}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range res.Deltas {
		// Messages should be structural, no emoji 🚨 ✨ or prose
		if containsEmoji(d.Message) {
			t.Errorf("Message contains emoji: %s (bug #29)", d.Message)
		}
		if containsProseWords(d.Message) {
			t.Errorf("Message contains prose words: %s (bug #29)", d.Message)
		}

		// Expected/Actual should use canonical sentinels: "<absent>" not "<non-existent>" or "<missing>"
		if d.Expected == "<non-existent>" || d.Expected == "<missing>" {
			t.Errorf("Expected uses non-canonical sentinel: %s (bug #29)", d.Expected)
		}
		if d.Actual == "<non-existent>" || d.Actual == "<missing>" {
			t.Errorf("Actual uses non-canonical sentinel: %s (bug #29)", d.Actual)
		}
	}
}

// Helpers for structural delta tests
func containsEmoji(s string) bool {
	for _, r := range s {
		if r >= 0x1F600 && r <= 0x1F64F { // emoticons
			return true
		}
		if r >= 0x1F300 && r <= 0x1F5FF { // misc symbols
			return true
		}
		if r >= 0x1F680 && r <= 0x1F6FF { // transport
			return true
		}
		if r >= 0x2600 && r <= 0x26FF { // misc symbols
			return true
		}
		if r >= 0x2700 && r <= 0x27BF { // dingbats
			return true
		}
	}
	return false
}

func containsProseWords(s string) bool {
	prose := []string{"Removed field", "Added field", "Type mismatch", "Nullability contract violation", "Missing property", "TYPE MISMATCH"}
	for _, w := range prose {
		if len(s) >= len(w) && (s == w || len(s) > len(w) && (s[:len(w)] == w || s[len(s)-len(w):] == w)) {
			return true
		}
	}
	return false
}

// Additional: nested object test
func TestNestedObjectDiff(t *testing.T) {
	baseline := `{"user": {"id": 1, "name": "Alice"}}`
	current := `{"user": {"id": "1", "name": "Alice"}}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	if !res.HasBreakingChanges {
		t.Errorf("expected breaking for nested type mismatch")
	}

	found := false
	for _, d := range res.Deltas {
		if d.JSONPath == "$.user.id" && d.Kind == types.KindTypeMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected nested field delta")
	}
}

// Additional: array item mismatch test
func TestArrayItemMismatch(t *testing.T) {
	baseline := `{"items": [{"id": 1}, {"id": 2}]}`
	current := `{"items": [{"id": "1"}, {"id": "2"}]}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	if !res.HasBreakingChanges {
		t.Errorf("expected breaking for array item type mismatch")
	}

	found := false
	for _, d := range res.Deltas {
		if d.JSONPath == "$.items[*].id" && d.Kind == types.KindTypeMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected array item delta")
	}
}

// Additional: added field should be INFO
func TestAddedField_IsInfo(t *testing.T) {
	baseline := `{"id": 1}`
	current := `{"id": 1, "new_field": "value"}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	if res.HasBreakingChanges {
		t.Errorf("added field should not be breaking")
	}

	found := false
	for _, d := range res.Deltas {
		if d.JSONPath == "$.new_field" && d.Severity == types.SeverityInfo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected INFO severity for added field")
	}
}

// Additional: special-character path test
func TestSpecialCharPaths(t *testing.T) {
	baseline := `{"a.b": 1, "a:b": 2}`
	current := `{"a.b": "changed", "a:b": 2}`

	res, err := CompareJSON(baseline, current)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range res.Deltas {
		// Both keys contain special chars, should be bracket-quoted
		if d.JSONPath == "$.a.b" || d.JSONPath == "$.a:b" {
			t.Errorf("special char key should be bracket-quoted, got %s (bug #25)", d.JSONPath)
		}
	}
}