package schema

import (
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

func TestInfer_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType types.JSONNodeType
	}{
		{"string", `"hello"`, types.TypeString},
		{"integer", `42`, types.TypeInteger},
		{"number", `3.14`, types.TypeNumber},
		{"boolean", `true`, types.TypeBoolean},
		{"null", `null`, types.TypeNull},
		{"array", `[]`, types.TypeArray},
		{"object", `{}`, types.TypeObject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := InferFromJSON(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if node.Type != tt.wantType {
				t.Errorf("type = %v, want %v", node.Type, tt.wantType)
			}
		})
	}
}

func TestInfer_ObjectWithRequiredKeys(t *testing.T) {
	node, err := InferFromJSON(`{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatal(err)
	}
	if node.Type != types.TypeObject {
		t.Fatalf("expected object, got %v", node.Type)
	}
	if len(node.Properties) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(node.Properties))
	}
	if !containsString(node.RequiredKeys, "name") || !containsString(node.RequiredKeys, "age") {
		t.Errorf("required keys = %v, want [name age]", node.RequiredKeys)
	}
}

func TestInfer_ArrayItems(t *testing.T) {
	node, err := InferFromJSON(`[1, 2, 3]`)
	if err != nil {
		t.Fatal(err)
	}
	if node.Type != types.TypeArray {
		t.Fatalf("expected array, got %v", node.Type)
	}
	if node.ItemSchema == nil {
		t.Fatal("expected item schema")
	}
	if node.ItemSchema.Type != types.TypeInteger {
		t.Errorf("item type = %v, want integer", node.ItemSchema.Type)
	}
}

func TestInfer_EmptyArray_UnknownItem(t *testing.T) {
	node, err := InferFromJSON(`[]`)
	if err != nil {
		t.Fatal(err)
	}
	if node.ItemSchema == nil {
		t.Fatal("expected item schema for empty array")
	}
	if node.ItemSchema.Type != types.TypeUnknown {
		t.Errorf("empty array item type = %v, want unknown", node.ItemSchema.Type)
	}
}

// --- PR #4 tests ---

// #34: String format detection (date, date-time, uuid, email)
func TestStringFormatDetection(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFormat string
	}{
		{"date", `"2024-01-15"`, "date"},
		{"date-time", `"2024-01-15T10:30:00Z"`, "date-time"},
		{"uuid", `"550e8400-e29b-41d4-a716-446655440000"`, "uuid"},
		{"email", `"user@example.com"`, "email"},
		{"no-format", `"plain string"`, ""},
		{"invalid-uuid", `"not-a-uuid"`, ""},
		{"invalid-email", `"not-an-email"`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := InferFromJSON(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if string(node.Format) != tt.wantFormat {
				t.Errorf("format = %q, want %q", node.Format, tt.wantFormat)
			}
		})
	}
}

// #31: mergeNodes should not alias pointers (defensive copy)
func TestMergeNodes_NoAliasing(t *testing.T) {
	a := Infer(map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Alice",
			"age":  30,
		},
	})
	b := Infer(map[string]interface{}{
		"user": map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		},
	})

	merged := mergeNodes(a, b)

	// Verify the merged result has both fields
	user := merged.Properties["user"]
	if user == nil {
		t.Fatal("expected merged user object")
	}

	// Check that original 'a' is NOT mutated by merge
	// Original 'a' should only have name and age
	if a.Properties["user"].Properties["email"] != nil {
		t.Errorf("original 'a' was mutated by mergeNodes (aliasing bug #31)")
	}

	// Check that original 'b' is NOT mutated
	if b.Properties["user"].Properties["age"] != nil {
		t.Errorf("original 'b' was mutated by mergeNodes (aliasing bug #31)")
	}

	// Merged should have all fields from both
	if user.Properties["name"] == nil {
		t.Error("merged missing name")
	}
	if user.Properties["email"] == nil {
		t.Error("merged missing email")
	}
	if user.Properties["age"] == nil {
		t.Error("merged missing age")
	}
}

// #34: format field exists on JSONSchemaNode
func TestSchemaNode_HasFormatField(t *testing.T) {
	node := &types.JSONSchemaNode{
		Type:   types.TypeString,
		Format: "date",
	}
	if node.Format != "date" {
		t.Errorf("Format field not working: %v", node.Format)
	}
}

// Additional: integer/number merge
func TestMergeNodes_IntegerNumberMerge(t *testing.T) {
	a := Infer(map[string]interface{}{"value": 42})
	b := Infer(map[string]interface{}{"value": 3.14})

	merged := mergeNodes(a, b)

	if merged.Properties["value"] == nil {
		t.Fatal("expected merged value")
	}
	// Should promote to number
	if merged.Properties["value"].Type != types.TypeNumber {
		t.Errorf("integer+number merge = %v, want number", merged.Properties["value"].Type)
	}
}

// Additional: null merges properly
func TestMergeNodes_NullHandling(t *testing.T) {
	a := Infer(map[string]interface{}{"value": 42})
	b := Infer(map[string]interface{}{"value": nil})

	merged := mergeNodes(a, b)

	if merged.Properties["value"] == nil {
		t.Fatal("expected merged value")
	}
	if !merged.Properties["value"].Nullable {
		t.Error("null merge should set nullable=true")
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}