package diff

import (
	"testing"
)

func TestIsNoiseKey(t *testing.T) {
	noiseKeys := []string{
		"request_id", "requestId", "req_id", "reqId",
		"trace_id", "traceId", "correlation_id", "span_id",
		"timestamp", "time", "created_at", "updated_at", "server_time",
		"etag", "nonce",
	}

	for _, key := range noiseKeys {
		if !IsNoiseKey(key) {
			t.Errorf("Expected '%s' to be classified as noise key, but got false", key)
		}
	}

	validKeys := []string{
		"id", "user_id", "username", "email", "status", "roles", "total_count",
	}

	for _, key := range validKeys {
		if IsNoiseKey(key) {
			t.Errorf("Expected '%s' NOT to be noise key, but got true", key)
		}
	}
}

func TestNormalizerNoiseFilteringInDiff(t *testing.T) {
	baseJSON := `{"id": 1, "username": "alex", "request_id": "abc-123", "created_at": 1690000000}`
	currJSON := `{"id": 1, "username": "alex", "request_id": "xyz-789", "created_at": 1690005555}`

	diff, err := CompareJSON(baseJSON, currJSON)
	if err != nil {
		t.Fatalf("CompareJSON failed: %v", err)
	}

	if diff.HasBreakingChanges {
		t.Errorf("Expected no breaking changes for dynamic noise key value shifts, but got breaking changes")
	}

	if len(diff.Deltas) > 0 {
		t.Errorf("Expected 0 deltas after noise filtering, got %d deltas", len(diff.Deltas))
	}
}
