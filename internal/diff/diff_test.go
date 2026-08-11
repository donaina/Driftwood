package diff

import (
	"testing"

	"github.com/callmidavid/apidiff/pkg/types"
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
