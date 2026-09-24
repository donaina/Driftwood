package openapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/netguard"
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
					"id":    {Type: "integer"},
					"name":  {Type: "string"},
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
			"Base":     {Type: "object", Properties: map[string]*Schema{"id": {Type: "integer"}}},
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
	err = s.ImportToStorage("acme", store)
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

	// Under the project the caller named. An import that resolved the project
	// for itself would file a client's API under whichever project happened to
	// be on screen.
	if got := store.projects[key]; got != "acme" {
		t.Errorf("imported %s under project %q, want acme", key, got)
	}

	// An imported spec is a declared contract, so it must not land as
	// provisional — the dashboard would otherwise ask the user to confirm the
	// document they just handed us.
	if got := store.sources[key]; got != types.BaselineSourceSpec {
		t.Errorf("source for %s = %q, want %q", key, got, types.BaselineSourceSpec)
	}

	// The document's own schema must reach storage rather than being regenerated
	// from the sample. The sample is generated *from* this node, so inference
	// over it recovers less than the node holds — the `required` list above being
	// the part that matters, since a property it omits is a property the API is
	// allowed to stop sending.
	declared, ok := store.schemas[key]
	if !ok {
		t.Fatalf("the parsed schema for %s was not passed through to storage", key)
	}
	// This fixture's spec declares no `required` list, so the answer is empty.
	// That emptiness is the evidence: inference over the generated sample would
	// have reported every key it wrote, ["id", "name"], so a non-empty list here
	// would mean the declared schema had been regenerated from the sample.
	if len(declared.RequiredKeys) != 0 {
		t.Errorf("required keys = %v, want [] — the document declared none, but inference over the sample would say [id name]",
			declared.RequiredKeys)
	}
}

// A spec that declares a required list and a format is the case the two cannot
// be told apart in: inference over the generated sample marks every key it wrote
// as required, which is a stronger claim than the document made.
func TestImportToStorage_PreservesDeclaredRequiredAndFormat(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "T", "version": "1"},
		"paths": {
			"/users/{id}": {
				"get": {
					"responses": {
						"200": {
							"description": "ok",
							"content": {
								"application/json": {
									"schema": {
										"type": "object",
										"required": ["id"],
										"properties": {
											"id": {"type": "integer"},
											"nickname": {"type": "string"},
											"joined": {"type": "string", "format": "date-time"}
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
	store := &mockStore{baselines: make(map[string]string)}
	if err := s.ImportToStorage("acme", store); err != nil {
		t.Fatalf("ImportToStorage failed: %v", err)
	}

	declared := store.schemas["GET:/users/{id}"]
	if declared == nil {
		t.Fatal("no schema was stored")
	}
	if got := declared.Properties["joined"].Format; got != "date-time" {
		t.Errorf("joined format = %q, want date-time from the document", got)
	}
	required := map[string]bool{}
	for _, k := range declared.RequiredKeys {
		required[k] = true
	}
	if !required["id"] || required["nickname"] || required["joined"] {
		t.Errorf("required = %v, want only [id] as the document declared", declared.RequiredKeys)
	}
}

type mockStore struct {
	baselines map[string]string
	sources   map[string]string
	schemas   map[string]*types.JSONSchemaNode
	// projects records which project each save was addressed to, keyed the same
	// way, so a test can assert an import landed under the project it named
	// rather than under whatever the store would have picked by itself.
	projects map[string]string
}

func (m *mockStore) SaveBaselineWithSchema(projectID, method, path, samplePayload string, declared *types.JSONSchemaNode, source string) (*types.ContractBaseline, error) {
	m.baselines[method+":"+path] = samplePayload
	if m.projects == nil {
		m.projects = map[string]string{}
	}
	m.projects[method+":"+path] = projectID
	if m.sources == nil {
		m.sources = map[string]string{}
	}
	m.sources[method+":"+path] = source
	if declared != nil {
		if m.schemas == nil {
			m.schemas = map[string]*types.JSONSchemaNode{}
		}
		m.schemas[method+":"+path] = declared
	}
	return &types.ContractBaseline{
		Method: method, Path: path, SamplePayload: samplePayload, Source: source,
	}, nil
}

/* The fetch path, which until now had no test at all: LoadFromURL was a bare
   http.Get, so nothing here could have failed for a reason the compiler would
   have caught. These are the properties that request did not have. */

const fetchSpec = `{"openapi":"3.0.0","info":{"title":"Fetched","version":"1.0.0"},"paths":{}}`

// TestLoadFromURLAcceptsTheOperatorsOwnURL is the baseline: the ordinary case
// still works, so the rest of these are not passing because everything fails.
func TestLoadFromURLAcceptsTheOperatorsOwnURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fetchSpec))
	}))
	defer srv.Close()

	spec, err := LoadFromURL(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("LoadFromURL: %v", err)
	}
	if spec.Info.Title != "Fetched" {
		t.Errorf("title = %q, want %q", spec.Info.Title, "Fetched")
	}
}

// TestLoadFromURLHasADeadline is the regression for the bare http.Get. A server
// that accepts the connection and then says nothing used to hold the command
// open forever; the only thing that ends it now is the client's own deadline.
func TestLoadFromURLHasADeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // never answers until the test is over
	}))
	defer func() { close(release); srv.Close() }()

	old := specFetchTimeout
	specFetchTimeout = 200 * time.Millisecond
	defer func() { specFetchTimeout = old }()

	start := time.Now()
	if _, err := LoadFromURL(srv.URL); err == nil {
		t.Fatal("LoadFromURL returned no error from a server that never answers")
	}
	// Generous against a loaded machine, and still four orders of magnitude
	// short of waiting out a request that has no deadline at all.
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("took %v to give up on an unanswering server", elapsed)
	}
}

// TestLoadFromURLFollowsASameHostRedirect keeps the ordinary http-to-https
// upgrade working, which is why this policy is narrower here than the webhook
// deliverer's outright refusal.
func TestLoadFromURLFollowsASameHostRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spec.json" {
			http.Redirect(w, r, "/moved/openapi.json", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(fetchSpec))
	}))
	defer srv.Close()

	spec, err := LoadFromURL(srv.URL + "/spec.json")
	if err != nil {
		t.Fatalf("LoadFromURL: %v", err)
	}
	if spec.Info.Title != "Fetched" {
		t.Errorf("title = %q; the same-host redirect was not followed", spec.Info.Title)
	}
}

// TestLoadFromURLRefusesAnotherHostsRedirect is the SSRF case: the operator
// named one host, and a redirect must not turn that into a request to a host
// nobody named. Asserted by the second server's own request count, because a
// refusal that still connects is not a refusal.
func TestLoadFromURLRefusesAnotherHostsRedirect(t *testing.T) {
	var hits int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(fetchSpec))
	}))
	defer elsewhere.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/openapi.json", http.StatusFound)
	}))
	defer origin.Close()

	_, err := LoadFromURL(origin.URL)
	if err == nil {
		t.Fatal("LoadFromURL followed a redirect to a host the operator did not name")
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("the redirect target received %d request(s); a refused redirect must not connect", n)
	}
	if !strings.Contains(err.Error(), "which is not") {
		t.Errorf("error = %v, want it to name the host it refused to follow to", err)
	}
}

// TestLoadFromURLReportsANonOKStatus is the 404-shaped case. The body of an
// error page used to reach the JSON parser and come back as "invalid character
// '<' looking for beginning of value", which describes the symptom.
func TestLoadFromURLReportsANonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>not found</html>", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := LoadFromURL(srv.URL + "/openapi.json")
	if err == nil {
		t.Fatal("LoadFromURL accepted a 404 as a spec")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want it to report the status the server answered with", err)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Errorf("error = %v, want the cause (the status) and not the parse symptom", err)
	}
}

// TestLoadFromURLBoundsTheResponseSize pins the cap. The body is streamed
// rather than allocated so the test costs a transfer, not a second 32 MiB.
func TestLoadFromURLBoundsTheResponseSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.CopyN(w, neverEnding('x'), maxSpecBytes+1)
	}))
	defer srv.Close()

	_, err := LoadFromURL(srv.URL)
	if err == nil {
		t.Fatal("LoadFromURL accepted a response larger than maxSpecBytes")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want it to say the response exceeded the limit", err)
	}
}

// TestLoadFromURLRefusesAnUnusableURL keeps the URL judgement on this path, so
// a spec URL is held to the same shape as every other target the product dials.
func TestLoadFromURLRefusesAnUnusableURL(t *testing.T) {
	for _, raw := range []string{"", "   ", "ftp://example.com/spec.json", "https:///spec.json"} {
		if _, err := LoadFromURL(raw); err == nil {
			t.Errorf("LoadFromURL(%q) was accepted", raw)
		} else if !errors.Is(err, netguard.ErrInvalidTarget) {
			t.Errorf("LoadFromURL(%q) error = %v, want it to wrap ErrInvalidTarget", raw, err)
		}
	}
}

type repeatReader byte

func neverEnding(b byte) io.Reader { return repeatReader(b) }

func (r repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}
