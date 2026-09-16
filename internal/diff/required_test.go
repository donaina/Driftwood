package diff

import (
	"testing"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
)

func node(t *testing.T, payload string) *types.JSONSchemaNode {
	t.Helper()
	n, err := schema.InferFromJSON(payload)
	if err != nil {
		t.Fatalf("inferring %s: %v", payload, err)
	}
	return n
}

// deltaAt returns the single delta at path, failing if there is not exactly one.
func deltaAt(t *testing.T, d *types.ContractDiff, path string) types.DiffDelta {
	t.Helper()
	var found []types.DiffDelta
	for _, delta := range d.Deltas {
		if delta.JSONPath == path {
			found = append(found, delta)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly 1 delta at %s, got %d (all deltas: %+v)", path, len(found), d.Deltas)
	}
	return found[0]
}

// A baseline inferred from a payload marks every key it saw as required, so a
// removal still reads as breaking. This is the behaviour that existed before
// RequiredKeys was consulted, and it must not regress: a genuinely mandatory
// field vanishing is the case the product exists to catch.
func TestRemovedField_AgainstInferredBaseline_IsBreaking(t *testing.T) {
	d, err := CompareJSON(`{"id": 1, "username": "alex"}`, `{"id": 1}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.username")
	if delta.Severity != types.SeverityBreaking {
		t.Errorf("severity = %s, want BREAKING", delta.Severity)
	}
	if !d.HasBreakingChanges {
		t.Error("HasBreakingChanges = false")
	}
}

// The point of the phase: a spec says which properties it promises. One it did
// not promise leaving the payload is a change worth reporting, not a broken
// contract.
func TestRemovedField_OutsideTheDeclaredRequiredList_IsWarning(t *testing.T) {
	baseline := node(t, `{"id": 1, "username": "alex", "nickname": "al"}`)
	baseline.RequiredKeys = []string{"id"} // what the spec's `required` said

	d, err := CompareJSONWithSchema(baseline, `{"id": 1, "username": "alex"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	delta := deltaAt(t, d, "$.nickname")
	if delta.Severity != types.SeverityWarning {
		t.Errorf("severity = %s, want WARNING", delta.Severity)
	}
	if delta.Kind != types.KindRemovedField {
		t.Errorf("kind = %s, want REMOVED_FIELD", delta.Kind)
	}
	if d.HasBreakingChanges {
		t.Error("HasBreakingChanges = true: an optional field leaving is not a broken contract")
	}
	if !d.HasWarnings {
		t.Error("HasWarnings = false: the removal was not reported at any tier")
	}
}

func TestRemovedField_InsideTheDeclaredRequiredList_IsBreaking(t *testing.T) {
	baseline := node(t, `{"id": 1, "username": "alex", "nickname": "al"}`)
	baseline.RequiredKeys = []string{"id", "username"}

	d, err := CompareJSONWithSchema(baseline, `{"id": 1, "nickname": "al"}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	if got := deltaAt(t, d, "$.username").Severity; got != types.SeverityBreaking {
		t.Errorf("$.username severity = %s, want BREAKING", got)
	}
	if !d.HasBreakingChanges {
		t.Error("HasBreakingChanges = false")
	}
}

// An object the spec gave no `required` list for is the shape
// openapi.ImportToStorage produces: RequiredKeys is empty, not absent. Nothing
// is known to be promised, so nothing can be reported as a broken promise.
func TestRemovedField_AgainstASpecWithNoRequiredList_IsWarning(t *testing.T) {
	baseline := node(t, `{"id": 1, "username": "alex"}`)
	baseline.RequiredKeys = []string{}

	d, err := CompareJSONWithSchema(baseline, `{}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	for _, delta := range d.Deltas {
		if delta.Severity == types.SeverityBreaking {
			t.Errorf("%s reported BREAKING, but the baseline declared nothing required", delta.JSONPath)
		}
	}
	if !d.HasWarnings {
		t.Error("HasWarnings = false: the removals were not reported at any tier")
	}
}

// The hazard this guards: CompareJSON re-infers the baseline from SamplePayload
// and so replaces a spec's required list with "every key in the sample". Given
// the same payloads, that path must NOT be the one the proxy takes — which is
// why CompareJSONWithSchema exists. If these two ever agree on this input,
// the stored schema is not being consulted.
func TestCompareJSONWithSchema_DoesNotReInferTheBaseline(t *testing.T) {
	payload := `{"id": 1, "username": "alex", "nickname": "al"}`
	current := `{"id": 1, "username": "alex"}`

	reInferred, err := CompareJSON(payload, current)
	if err != nil {
		t.Fatalf("CompareJSON: %v", err)
	}
	if !reInferred.HasBreakingChanges {
		t.Fatal("CompareJSON no longer treats the removal as breaking; this test's premise is gone")
	}

	baseline := node(t, payload)
	baseline.RequiredKeys = []string{"id"}

	withSchema, err := CompareJSONWithSchema(baseline, current)
	if err != nil {
		t.Fatalf("CompareJSONWithSchema: %v", err)
	}
	if withSchema.HasBreakingChanges {
		t.Error("CompareJSONWithSchema reported BREAKING for a field the baseline did not require — " +
			"it is re-inferring the baseline from the sample instead of using the schema it was given")
	}
}

// Requiredness is a property of the object that holds the key, not of the tree,
// so a nested object's own required list governs its own keys.
func TestRemovedField_RequirednessIsPerObject(t *testing.T) {
	baseline := node(t, `{"id": 1, "profile": {"handle": "al", "bio": "hi"}}`)
	baseline.RequiredKeys = []string{"id"}
	baseline.Properties["profile"].RequiredKeys = []string{"handle"}

	d, err := CompareJSONWithSchema(baseline, `{"id": 1, "profile": {"handle": "al"}}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	if got := deltaAt(t, d, "$.profile.bio").Severity; got != types.SeverityWarning {
		t.Errorf("$.profile.bio severity = %s, want WARNING", got)
	}
	if d.HasBreakingChanges {
		t.Error("HasBreakingChanges = true")
	}

	// The same nesting, with the nested key required.
	baseline2 := node(t, `{"id": 1, "profile": {"handle": "al", "bio": "hi"}}`)
	baseline2.RequiredKeys = []string{"id"}
	baseline2.Properties["profile"].RequiredKeys = []string{"handle", "bio"}

	d2, err := CompareJSONWithSchema(baseline2, `{"id": 1, "profile": {"handle": "al"}}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if got := deltaAt(t, d2, "$.profile.bio").Severity; got != types.SeverityBreaking {
		t.Errorf("$.profile.bio severity = %s, want BREAKING", got)
	}
}

func TestCompareJSONWithSchema_NilBaselineIsAnError(t *testing.T) {
	if _, err := CompareJSONWithSchema(nil, `{"id": 1}`); err == nil {
		t.Error("a nil baseline schema was accepted")
	}
}

// The optionality gate must not swallow the other verdicts: a type change to a
// field outside the required list is still breaking, because the spec said the
// field would be there if it appeared at all.
func TestTypeMismatch_IsBreakingEvenForAnOptionalField(t *testing.T) {
	baseline := node(t, `{"id": 1, "nickname": "al"}`)
	baseline.RequiredKeys = []string{"id"}

	d, err := CompareJSONWithSchema(baseline, `{"id": 1, "nickname": 42}`)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	if got := deltaAt(t, d, "$.nickname").Severity; got != types.SeverityBreaking {
		t.Errorf("$.nickname severity = %s, want BREAKING", got)
	}
}
