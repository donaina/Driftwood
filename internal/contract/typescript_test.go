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
	if !strings.Contains(tsCode, "interface WrapperUser") {
		t.Errorf("nested object 'user' should become named interface 'WrapperUser', got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "interface WrapperMetadata") {
		t.Errorf("nested object 'metadata' should become named interface 'WrapperMetadata', got:\n%s", tsCode)
	}
	// Main interface should reference the named interfaces
	if !strings.Contains(tsCode, "metadata: WrapperMetadata;") {
		t.Errorf("main interface should reference WrapperMetadata, got:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "user: WrapperUser;") {
		t.Errorf("main interface should reference WrapperUser, got:\n%s", tsCode)
	}
}

// Nested type names are scoped to the interface that owns them. Naming them
// after the key alone meant two endpoints that both have a `user` object each
// declared a top-level `interface User`, and one .d.ts holding two disagreeing
// declarations of a name does not compile. The exporter makes the root names
// unique; this is what makes everything under them unique too.
func TestTypeScript_NestedNamesAreScoped(t *testing.T) {
	node, _ := schema.InferFromJSON(`{"user": {"id": 1}, "orders": [{"ref": "a"}]}`)

	tsCode := GenerateTypeScriptInterfaces("GetUsersResponse", node)

	for _, want := range []string{"interface GetUsersResponseUser", "interface GetUsersResponseOrdersItem"} {
		if !strings.Contains(tsCode, want) {
			t.Errorf("expected %q, got:\n%s", want, tsCode)
		}
	}
	if strings.Contains(tsCode, "interface User") || strings.Contains(tsCode, "interface OrdersItem") {
		t.Errorf("a nested type was declared under its bare key name, which collides across endpoints:\n%s", tsCode)
	}
}

// Optionality comes from the schema's own required list. Before this, the only
// signal read was Nullable, so every property of a spec that declares
// `required` was emitted as mandatory and a consumer of the generated file was
// told to expect fields the API may omit.
func TestTypeScript_RequiredKeysDriveOptionality(t *testing.T) {
	node := &types.JSONSchemaNode{
		Type: types.TypeObject,
		Properties: map[string]*types.JSONSchemaNode{
			"id":       {Type: types.TypeInteger},
			"nickname": {Type: types.TypeString},
		},
		RequiredKeys: []string{"id"},
	}

	tsCode := GenerateTypeScriptInterfaces("Profile", node)

	if !strings.Contains(tsCode, "id: number;") {
		t.Errorf("a property in the required list was not emitted as mandatory:\n%s", tsCode)
	}
	if !strings.Contains(tsCode, "nickname?: string;") {
		t.Errorf("a property outside the required list was not marked optional:\n%s", tsCode)
	}
}

// An object whose required list we do not have falls back to optional. That is
// the direction that fails safe: a consumer is made to handle a field that is
// always there, rather than to assume one that may be missing.
func TestTypeScript_AbsentRequiredListFailsSafe(t *testing.T) {
	node := &types.JSONSchemaNode{
		Type: types.TypeObject,
		Properties: map[string]*types.JSONSchemaNode{
			"id": {Type: types.TypeInteger},
		},
	}

	tsCode := GenerateTypeScriptInterfaces("Unknown", node)

	if !strings.Contains(tsCode, "id?: number;") {
		t.Errorf("with no required list, a property should be optional, got:\n%s", tsCode)
	}
}

// The same rule applies inside an object that is inlined rather than extracted
// into its own interface, which is a second, separate code path.
func TestTypeScript_RequiredKeysDriveOptionalityInInlinedObjects(t *testing.T) {
	node := &types.JSONSchemaNode{
		Type: types.TypeObject,
		Properties: map[string]*types.JSONSchemaNode{
			"outer": {
				Type: types.TypeArray,
				ItemSchema: &types.JSONSchemaNode{
					Type: types.TypeObject,
					Properties: map[string]*types.JSONSchemaNode{
						"a": {Type: types.TypeInteger},
						"b": {Type: types.TypeInteger},
					},
					RequiredKeys: []string{"a"},
				},
			},
		},
		RequiredKeys: []string{"outer"},
	}

	tsCode := GenerateTypeScriptInterfaces("Inline", node)

	if !strings.Contains(tsCode, "a: number") || !strings.Contains(tsCode, "b?: number") {
		t.Errorf("an inlined object ignored its required list:\n%s", tsCode)
	}
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
	if !strings.Contains(tsCode, "interface ResponseItemsItem") {
		t.Errorf("array of objects should extract item interface 'ResponseItemsItem', got:\n%s", tsCode)
	}
	// Main interface should reference the named item interface
	if !strings.Contains(tsCode, "items: ResponseItemsItem[];") {
		t.Errorf("main interface should reference ResponseItemsItem[], got:\n%s", tsCode)
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
