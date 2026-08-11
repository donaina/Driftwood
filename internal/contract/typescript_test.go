package contract

import (
	"strings"
	"testing"

	"github.com/callmidavid/apidiff/internal/schema"
)

func TestTypeScriptGeneration(t *testing.T) {
	jsonInput := `{"id": 1024, "username": "alex", "email": "alex@dev.com", "roles": ["admin"]}`

	node, err := schema.InferFromJSON(jsonInput)
	if err != nil {
		t.Fatalf("failed to infer schema: %v", err)
	}

	tsCode := GenerateTypeScriptInterfaces("UserResponse", node)

	if !strings.Contains(tsCode, "export interface UserResponse") {
		t.Errorf("expected TS output to contain export interface UserResponse, got:\n%s", tsCode)
	}

	if !strings.Contains(tsCode, "id: number;") {
		t.Errorf("expected id: number;, got:\n%s", tsCode)
	}

	if !strings.Contains(tsCode, "username: string;") {
		t.Errorf("expected username: string;, got:\n%s", tsCode)
	}

	if !strings.Contains(tsCode, "roles: string[];") {
		t.Errorf("expected roles: string[];, got:\n%s", tsCode)
	}
}
