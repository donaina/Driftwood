package openapi

import (
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

func TestLoadFromBytes(t *testing.T) {
	const spec = `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"type": "object",
											"properties": {
												"id": {"type": "integer"},
												"name": {"type": "string"},
												"email": {"type": "string", "format": "email"}
											},
											"required": ["id", "name"]
										}
									}
								}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"User": {
					"type": "object",
					"properties": {
						"id": {"type": "integer"},
						"name": {"type": "string"}
					}
				}
			}
		}
	}`

	s, err := LoadFromBytes([]byte(spec))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}
	if s.Info.Title != "Test API" {
		t.Errorf("title = %s, want 'Test API'", s.Info.Title)
	}
	if len(s.Paths) != 1 {
		t.Errorf("paths = %d, want 1", len(s.Paths))
	}
}

func TestExtractContracts(t *testing.T) {
	const spec = `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {"$ref": "#/components/schemas/User"}
									}
								}
							}
						}
					}
				},
				"post": {
					"operationId": "createUser",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {"$ref": "#/components/schemas/User"}
							}
						}
					},
					"responses": {
						"201": {
							"description": "Created",
							"content": {
								"application/json": {"schema": {"$ref": "#/components/schemas/User"}}
							}
						}
					}
				}
			},
			"/users/{id}": {
				"get": {
					"operationId": "getUser",
					"parameters": [
						{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}
					],
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {"schema": {"$ref": "#/components/schemas/User"}}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"User": {
					"type": "object",
					"properties": {
						"id": {"type": "integer"},
						"name": {"type": "string"},
						"email": {"type": "string", "format": "email"}
					},
					"required": ["id", "name"]
				}
			}
		}
	}`

	s, err := LoadFromBytes([]byte(spec))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	contracts, err := s.ExtractContracts()
	if err != nil {
		t.Fatalf("ExtractContracts failed: %v", err)
	}

	// Should have 3 contracts: GET /users, POST /users, GET /users/{id}
	if len(contracts) != 3 {
		t.Errorf("contracts = %d, want 3", len(contracts))
	}

	// Check GET /users
	found := false
	for _, c := range contracts {
		if c.Method == "GET" && c.Path == "/users" {
			found = true
			if c.OperationID != "listUsers" {
				t.Errorf("operationId = %s, want 'listUsers'", c.OperationID)
			}
			if c.ResponseSchema == nil {
				t.Error("response schema should not be nil")
			} else if c.ResponseSchema.Type != types.TypeArray {
				t.Errorf("response type = %v, want array", c.ResponseSchema.Type)
			}
			if c.ResponseSchema.ItemSchema == nil {
				t.Error("item schema should not be nil")
			} else if c.ResponseSchema.ItemSchema.Properties == nil {
				t.Error("item schema properties should not be nil")
			}
		}
	}
	if !found {
		t.Error("GET /users contract not found")
	}

	// Check POST /users
	found = false
	for _, c := range contracts {
		if c.Method == "POST" && c.Path == "/users" {
			found = true
			if c.RequestSchema == nil {
				t.Error("request schema should not be nil")
			}
			if c.ResponseSchema == nil {
				t.Error("response schema should not be nil")
			}
		}
	}
	if !found {
		t.Error("POST /users contract not found")
	}

	// Check GET /users/{id}
	found = false
	for _, c := range contracts {
		if c.Method == "GET" && c.Path == "/users/{id}" {
			found = true
			if c.OperationID != "getUser" {
				t.Errorf("operationId = %s, want 'getUser'", c.OperationID)
			}
		}
	}
	if !found {
		t.Error("GET /users/{id} contract not found")
	}
}

func TestSchemaToNode(t *testing.T) {
	comps := Components{
		Schemas: map[string]*Schema{
			"User": {
				Type: "object",
				Properties: map[string]*Schema{
					"id": {Type: "integer"},
					"name": {Type: "string"},
					"email": {Type: "string", Format: "email"},
				},
				Required: []string{"id", "name"},
			},
		},
	}

	sch := &Schema{Ref: "#/components/schemas/User"}

	node, err := schemaToNode(sch, comps)
	if err != nil {
		t.Fatalf("schemaToNode failed: %v", err)
	}

	if node.Type != types.TypeObject {
		t.Errorf("type = %v, want object", node.Type)
	}
	if node.Properties == nil {
		t.Fatal("properties is nil")
	}
	if _, ok := node.Properties["id"]; !ok {
		t.Error("missing 'id' property")
	}
	if _, ok := node.Properties["email"]; !ok {
		t.Error("missing 'email' property")
	}
	if node.Properties["email"].Format != "email" {
		t.Errorf("email format = %s, want 'email'", node.Properties["email"].Format)
	}
	if len(node.RequiredKeys) != 2 {
		t.Errorf("required keys = %d, want 2", len(node.RequiredKeys))
	}
}

func TestSchemaToNode_Array(t *testing.T) {
	comps := Components{
		Schemas: map[string]*Schema{
			"Item": {Type: "integer"},
		},
	}

	sch := &Schema{
		Type:  "array",
		Items: &Schema{Ref: "#/components/schemas/Item"},
	}

	node, err := schemaToNode(sch, comps)
	if err != nil {
		t.Fatalf("schemaToNode failed: %v", err)
	}

	if node.Type != types.TypeArray {
		t.Errorf("type = %v, want array", node.Type)
	}
	if node.ItemSchema == nil {
		t.Fatal("item schema is nil")
	}
	if node.ItemSchema.Type != types.TypeInteger {
		t.Errorf("item type = %v, want integer", node.ItemSchema.Type)
	}
}

func TestSchemaToNode_AllOf(t *testing.T) {
	comps := Components{
		Schemas: map[string]*Schema{
			"Base": {Type: "object", Properties: map[string]*Schema{"id": {Type: "integer"}}},
			"Extended": {Type: "object", Properties: map[string]*Schema{"name": {Type: "string"}}},
		},
	}

	sch := &Schema{
		AllOf: []*Schema{
			{Ref: "#/components/schemas/Base"},
			{Ref: "#/components/schemas/Extended"},
		},
	}

	node, err := schemaToNode(sch, comps)
	if err != nil {
		t.Fatalf("schemaToNode failed: %v", err)
	}

	if node.Type != types.TypeObject {
		t.Errorf("type = %v, want object", node.Type)
	}
	// Current implementation takes first matching allOf (Base in this case)
	if _, ok := node.Properties["id"]; !ok {
		t.Error("missing 'id' from allOf (first match)")
	}
	// Note: allOf merging is not fully implemented - only first valid schema used
}

func TestResolveRef(t *testing.T) {
	comps := Components{
		Schemas: map[string]*Schema{
			"User": {Type: "object", Properties: map[string]*Schema{"id": {Type: "integer"}}},
		},
	}

	// Valid ref
	sch, err := resolveRef("#/components/schemas/User", comps)
	if err != nil {
		t.Fatalf("resolveRef failed: %v", err)
	}
	if sch == nil {
		t.Fatal("resolved schema is nil")
	}

	// Invalid ref - not found
	_, err = resolveRef("#/components/schemas/NotFound", comps)
	if err == nil {
		t.Error("expected error for missing schema")
	}

	// Invalid ref - external
	_, err = resolveRef("http://example.com/schema.json", comps)
	if err == nil {
		t.Error("expected error for external ref")
	}

	// Invalid ref - wrong component type
	_, err = resolveRef("#/components/responses/NotFound", comps)
	if err == nil {
		t.Error("expected error for wrong component type")
	}
}

func TestImportToStorage(t *testing.T) {
	const spec = `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/users": {
				"get": {
					"operationId": "listUsers",
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": {
										"type": "object",
										"properties": {
											"id": {"type": "integer"},
											"name": {"type": "string"}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	s, err := LoadFromBytes([]byte(spec))
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	// Mock store
	store := &mockStore{baselines: make(map[string]string)}
	err = s.ImportToStorage(store)
	if err != nil {
		t.Fatalf("ImportToStorage failed: %v", err)
	}

	// Should have saved baseline
	if len(store.baselines) != 1 {
		t.Errorf("baselines = %d, want 1", len(store.baselines))
	}

	key := "GET:/users"
	if _, ok := store.baselines[key]; !ok {
		t.Errorf("missing baseline for %s", key)
	}
}

type mockStore struct {
	baselines map[string]string
}

func (m *mockStore) SaveBaseline(method, path, samplePayload string) (*types.ContractBaseline, error) {
	m.baselines[method+":"+path] = samplePayload
	return &types.ContractBaseline{
		Method: method, Path: path, SamplePayload: samplePayload,
	}, nil
}