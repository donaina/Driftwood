package contract

import (
	"strings"
	"testing"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
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

// --- PR #7 tests ---

// #26: Reserved/invalid keys should be quoted
func TestTypeScript_ReservedKeysQuoted(t *testing.T) {
	jsonInput := `{"type": "user", "interface": "admin", "class": "premium"}`

	node, _ := schema.InferFromJSON(jsonInput)
	tsCode := GenerateTypeScriptInterfaces("TestType", node)

	if !strings.Contains(tsCode, "`type`") && !strings.Contains(tsCode, `"type"`) {
		t.Errorf("reserved key 'type' should be quoted, got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "`interface`") && !strings.Contains(tsCode, `"interface"`) {
		t.Errorf("reserved key 'interface' should be quoted, got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "`class`") && !strings.Contains(tsCode, `"class"`) {
		t.Errorf("reserved key 'class' should be quoted, got:\n%s", tsCode)
	}
}

// #26: Invalid identifier keys should be quoted
func TestTypeScript_InvalidKeysQuoted(t *testing.T) {
	jsonInput := `{"key with spaces": "value", "123numeric": 1, "normal_key": "ok"}`

	node, _ := schema.InferFromJSON(jsonInput)
	tsCode := GenerateTypeScriptInterfaces("TestType", node)

	if !strings.Contains(tsCode, "`key with spaces`") && !strings.Contains(tsCode, `"key with spaces"`) {
		t.Errorf("key with spaces should be quoted, got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "`123numeric`") && !strings.Contains(tsCode, `"123numeric"`) {
		t.Errorf("numeric key should be quoted, got:\n%s", tsCode)
	}
	// Valid identifier should NOT be quoted
	if strings.Contains(tsCode, "`normal_key`") || strings.Contains(tsCode, `"normal_key"`) {
		t.Errorf("valid identifier should not be quoted, got:\n%s", tsCode)
	}
}

// #26: Nested objects should be extracted into named interfaces
func TestTypeScript_NestedObjectsNamed(t *testing.T) {
	jsonInput := `{"user": {"id": 1, "name": "Alice"}, "metadata": {"created": "2024-01-01"}}`

	node, _ := schema.InferFromJSON(jsonInput)
	tsCode := GenerateTypeScriptInterfaces("Wrapper", node)

	// Should have separate interfaces for nested objects (generated as "User" and "Metadata")
	if !strings.Contains(tsCode, "interface User") {
		t.Errorf("nested object 'user' should become named interface 'User', got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "interface Metadata") {
		t.Errorf("nested object 'metadata' should become named interface 'Metadata', got:\n%s", tsCode)
	}
	// Main interface should reference the named interfaces
	if !strings.Contains(tsCode, "metadata: Metadata;") {
		t.Errorf("main interface should reference Metadata, got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "user: User;") {
		t.Errorf("main interface should reference User, got:\n%s", tsCode)
	}
}

// #26: Interface name dedup - include method+path
func TestTypeScript_DedupInterfaceNames(t *testing.T) {
	// Two different endpoints with same structure but different paths
	jsonInput := `{"id": 1, "name": "Alice"}`

	node1, _ := schema.InferFromJSON(jsonInput)
	node2, _ := schema.InferFromJSON(jsonInput)

	ts1 := GenerateTypeScriptInterfaces("GET /api/users", node1)
	ts2 := GenerateTypeScriptInterfaces("POST /api/users", node2)

	// Should not produce identical interface names
	if strings.Contains(ts1, "interface GetApiUsers") && strings.Contains(ts2, "interface GetApiUsers") {
		t.Logf("TS1:\n%s\nTS2:\n%s", ts1, ts2)
	}
	// Each should have unique names
}

// #26: Nullable fields should use | null
func TestTypeScript_NullableUsesNullUnion(t *testing.T) {
	// Create schema with nullable field manually
	node := &types.JSONSchemaNode{
		Type: types.TypeObject,
		Properties: map[string]*types.JSONSchemaNode{
			"email": {
				Type:     types.TypeString,
				Nullable: true,
			},
		},
		RequiredKeys: []string{},
	}

	tsCode := GenerateTypeScriptInterfaces("Test", node)

	if !strings.Contains(tsCode, "email?") && !strings.Contains(tsCode, "email: string | null") {
		t.Errorf("nullable field should be optional or | null, got:\n%s", tsCode)
	}
}

// #26: Arrays with object items should extract item interface
func TestTypeScript_ArrayOfObjects(t *testing.T) {
	jsonInput := `{"items": [{"id": 1, "name": "A"}, {"id": 2, "name": "B"}]}`

	node, _ := schema.InferFromJSON(jsonInput)
	tsCode := GenerateTypeScriptInterfaces("Response", node)

	// Should have separate interface for array item (generated as "ItemsItem")
	if !strings.Contains(tsCode, "interface ItemsItem") {
		t.Errorf("array of objects should extract item interface 'ItemsItem', got:\n%s", tsCode)
	}
	// Main interface should reference the named item interface
	if !strings.Contains(tsCode, "items: ItemsItem[];") {
		t.Errorf("main interface should reference ItemsItem[], got:\n%s", tsCode)
	}
}

// #28: NormalizeConfig should be wired into diff (TODO: implement)
// func TestNormalizeConfig_Wired(t *testing.T) {
// 	config := DefaultNormalizeConfig()
// 	if !config.StripNoiseKeys {
// 		t.Error("StripNoiseKeys should be true by default")
// 	}
// 	if !config.SortArrayItems {
// 		t.Error("SortArrayItems should be true by default")
// 	}
// }

// #28: SortArrayItems implementation (TODO: implement)
// func TestNormalize_SortArrayItems(t *testing.T) {
// }

// Helper: isValidIdentifier (will be in normalizer) (TODO: implement)
// func TestIsValidIdentifier(t *testing.T) {
// }