package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/donaina/driftwood/internal/netguard"
	"github.com/donaina/driftwood/pkg/types"
)

// OpenAPISpec represents the top-level OpenAPI 3.x document
type OpenAPISpec struct {
	OpenAPI    string                 `json:"openapi"`
	Info       Info                   `json:"info"`
	Servers    []Server               `json:"servers"`
	Paths      map[string]PathItem    `json:"paths"`
	Components Components             `json:"components"`
	Raw        map[string]interface{} `json:"-"`
}

type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type PathItem map[string]Operation

type Operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Parameters  []Parameter         `json:"parameters"`
	RequestBody *RequestBody        `json:"requestBody"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description"`
	Required    bool    `json:"required"`
	Schema      *Schema `json:"schema"`
}

type RequestBody struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content"`
	Required    bool                 `json:"required"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema"`
}

type Schema struct {
	Type                 string             `json:"type"`
	Format               string             `json:"format"`
	Title                string             `json:"title"`
	Description          string             `json:"description"`
	Enum                 []interface{}      `json:"enum"`
	Const                interface{}        `json:"const"`
	Default              interface{}        `json:"default"`
	MultipleOf           *float64           `json:"multipleOf"`
	Maximum              *float64           `json:"maximum"`
	ExclusiveMaximum     interface{}        `json:"exclusiveMaximum"`
	Minimum              *float64           `json:"minimum"`
	ExclusiveMinimum     interface{}        `json:"exclusiveMinimum"`
	MaxLength            *int               `json:"maxLength"`
	MinLength            *int               `json:"minLength"`
	Pattern              string             `json:"pattern"`
	MaxItems             *int               `json:"maxItems"`
	MinItems             *int               `json:"minItems"`
	UniqueItems          bool               `json:"uniqueItems"`
	MaxProperties        *int               `json:"maxProperties"`
	MinProperties        *int               `json:"minProperties"`
	Required             []string           `json:"required"`
	Properties           map[string]*Schema `json:"properties"`
	AdditionalProperties interface{}        `json:"additionalProperties"`
	Items                *Schema            `json:"items"`
	AllOf                []*Schema          `json:"allOf"`
	AnyOf                []*Schema          `json:"anyOf"`
	OneOf                []*Schema          `json:"oneOf"`
	Not                  *Schema            `json:"not"`
	Ref                  string             `json:"$ref"`
}

type Components struct {
	Schemas         map[string]*Schema     `json:"schemas"`
	Responses       map[string]Response    `json:"responses"`
	Parameters      map[string]Parameter   `json:"parameters"`
	RequestBodies   map[string]RequestBody `json:"requestBodies"`
	SecuritySchemes map[string]interface{} `json:"securitySchemes"`
}

type EndpointContract struct {
	Method         string
	Path           string
	OperationID    string
	RequestSchema  *types.JSONSchemaNode
	ResponseSchema *types.JSONSchemaNode
}

func LoadFromFile(path string) (*OpenAPISpec, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return LoadFromBytes(data)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// These bound the one outbound request this package makes. The request used to
// be a bare http.Get, which has no timeout at all — a server that accepts the
// connection and then says nothing holds `drift import` open until the operator
// gives up — and an unbounded io.ReadAll, so the response body, not the spec,
// decided how much memory the command used.
const (
	// Generous by an order of magnitude: the largest OpenAPI documents in the
	// wild are a few megabytes, and this is a limit on what a hostile or broken
	// server can do to this process rather than a judgement about specs.
	maxSpecBytes = 32 << 20
	// Go's default policy follows ten hops. Five is already more than a spec URL
	// legitimately needs, and every hop has to stay on the origin host anyway.
	maxSpecRedirects = 5
)

// specFetchTimeout is a var rather than a const so a test can shrink it. There
// is no clock abstraction anywhere in this repo — the same reason
// webhook.backoffSchedule is a var — and without a seam the only way to test
// that this request has a deadline at all is to wait thirty seconds for one
// that does not. The deadline is the point: the request this replaced was a
// bare http.Get, which has none.
var specFetchTimeout = 30 * time.Second

// sameHostRedirects refuses a redirect that leaves the host the operator named.
//
// Go's default policy follows up to ten redirects to anywhere, which turns one
// operator-named URL into a request to a host nobody named. That is the same
// opening the webhook deliverer closes by refusing redirects outright, but this
// caller is different in a way that changes the right answer: a spec URL that
// upgrades http:// to https:// on the same host is ordinary and works, and the
// operator is sitting in front of the error if we refuse it.
//
// So the rule is narrower here rather than absent: the URL may redirect to
// itself, not to somebody else. The origin host is the one ParseAndValidate
// already judged, so staying on it means no hop reaches a host that was never
// checked.
func sameHostRedirects(origin string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxSpecRedirects {
			return fmt.Errorf("stopped after %d redirects", maxSpecRedirects)
		}
		if req.URL.Host != origin {
			return fmt.Errorf("redirects to %q, which is not %q — fetch that URL directly if you meant it", req.URL.Host, origin)
		}
		return nil
	}
}

// LoadFromURL fetches an OpenAPI document over the network.
//
// This is an operator command, not a request that arrived over the wire: the
// address came from a flag the operator typed on their own machine, so
// allowPrivate is true and ParseAndValidate judges the URL's shape rather than
// its reachability. That is the same reading of the same rule that lets
// `--target` point at http://localhost:3000 — see netguard.IsBlockedHost.
func LoadFromURL(raw string) (*OpenAPISpec, error) {
	parsed, err := netguard.ParseAndValidate(raw, true)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: specFetchTimeout,
		Transport: &http.Transport{
			DialContext:           netguard.DialContext(&net.Dialer{Timeout: 5 * time.Second}, true),
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: specFetchTimeout,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: sameHostRedirects(parsed.Host),
	}

	resp, err := client.Get(parsed.String())
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	/* A 404 or an error page otherwise reaches LoadFromBytes and comes back as
	   "invalid character '<' looking for beginning of value", which describes
	   the symptom and not the cause. */
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch URL: %s answered %s", parsed.Host, resp.Status)
	}

	// One byte past the cap, so a body exactly at the limit is accepted and one
	// over it is reported rather than silently truncated into a parse error.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxSpecBytes {
		return nil, fmt.Errorf("fetch URL: response from %s exceeds %d bytes", parsed.Host, maxSpecBytes)
	}
	return LoadFromBytes(data)
}

func LoadFromBytes(data []byte) (*OpenAPISpec, error) {
	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal OpenAPI: %w", err)
	}
	return &spec, nil
}

func (s *OpenAPISpec) ExtractContracts() ([]EndpointContract, error) {
	var contracts []EndpointContract

	for path, methodMap := range s.Paths {
		for method, op := range methodMap {
			httpMethod := strings.ToUpper(method)
			validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true, "HEAD": true, "OPTIONS": true}
			if !validMethods[httpMethod] {
				continue
			}

			var responseSchema *types.JSONSchemaNode
			for statusCode, resp := range op.Responses {
				if isSuccessStatus(statusCode) && resp.Content != nil {
					for _, mt := range resp.Content {
						if mt.Schema != nil {
							node, err := schemaToNode(mt.Schema, s.Components)
							if err != nil {
								return nil, fmt.Errorf("parse response schema for %s %s: %w", httpMethod, path, err)
							}
							responseSchema = node
							break
						}
					}
				}
				if responseSchema != nil {
					break
				}
			}

			var requestSchema *types.JSONSchemaNode
			if op.RequestBody != nil {
				for _, mt := range op.RequestBody.Content {
					if mt.Schema != nil {
						node, err := schemaToNode(mt.Schema, s.Components)
						if err != nil {
							return nil, fmt.Errorf("parse request schema for %s %s: %w", httpMethod, path, err)
						}
						requestSchema = node
						break
					}
				}
			}

			contracts = append(contracts, EndpointContract{
				Method:         httpMethod,
				Path:           path,
				OperationID:    op.OperationID,
				RequestSchema:  requestSchema,
				ResponseSchema: responseSchema,
			})
		}
	}

	return contracts, nil
}

func isSuccessStatus(code string) bool {
	if len(code) >= 3 && code[0] == '2' {
		return true
	}
	switch code {
	case "200", "201", "202", "204":
		return true
	}
	return false
}

func schemaToNode(sch *Schema, comps Components) (*types.JSONSchemaNode, error) {
	if sch.Ref != "" {
		resolved, err := resolveRef(sch.Ref, comps)
		if err != nil {
			return nil, err
		}
		return schemaToNode(resolved, comps)
	}

	node := &types.JSONSchemaNode{
		Nullable:     false,
		RequiredKeys: []string{},
	}

	switch sch.Type {
	case "string":
		node.Type = types.TypeString
		node.Format = sch.Format
	case "integer":
		node.Type = types.TypeInteger
	case "number":
		node.Type = types.TypeNumber
	case "boolean":
		node.Type = types.TypeBoolean
	case "array":
		node.Type = types.TypeArray
		if sch.Items != nil {
			itemNode, err := schemaToNode(sch.Items, comps)
			if err != nil {
				return nil, err
			}
			node.ItemSchema = itemNode
		}
	case "object":
		node.Type = types.TypeObject
		if sch.Properties != nil {
			node.Properties = make(map[string]*types.JSONSchemaNode)
			for k, v := range sch.Properties {
				propNode, err := schemaToNode(v, comps)
				if err != nil {
					return nil, err
				}
				node.Properties[k] = propNode
			}
		}
		if sch.Required != nil {
			node.RequiredKeys = sch.Required
		}
	case "":
		if sch.Properties != nil {
			node.Type = types.TypeObject
			node.Properties = make(map[string]*types.JSONSchemaNode)
			for k, v := range sch.Properties {
				propNode, err := schemaToNode(v, comps)
				if err != nil {
					return nil, err
				}
				node.Properties[k] = propNode
			}
			if sch.Required != nil {
				node.RequiredKeys = sch.Required
			}
		} else if sch.Items != nil {
			node.Type = types.TypeArray
			itemNode, err := schemaToNode(sch.Items, comps)
			if err != nil {
				return nil, err
			}
			node.ItemSchema = itemNode
		} else {
			node.Type = types.TypeUnknown
		}
	default:
		node.Type = types.TypeUnknown
	}

	if sch.Format != "" && node.Format == "" {
		node.Format = sch.Format
	}

	if len(sch.AllOf) > 0 {
		for _, s := range sch.AllOf {
			if s != nil {
				n, err := schemaToNode(s, comps)
				if err == nil && n != nil {
					return n, nil
				}
			}
		}
	}
	if len(sch.AnyOf) > 0 {
		for _, s := range sch.AnyOf {
			if s != nil {
				n, err := schemaToNode(s, comps)
				if err == nil && n != nil {
					return n, nil
				}
			}
		}
	}
	if len(sch.OneOf) > 0 {
		for _, s := range sch.OneOf {
			if s != nil {
				n, err := schemaToNode(s, comps)
				if err == nil && n != nil {
					return n, nil
				}
			}
		}
	}

	return node, nil
}

func resolveRef(ref string, comps Components) (*Schema, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("external refs not supported: %s", ref)
	}

	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid ref format: %s", ref)
	}

	if parts[0] != "components" {
		return nil, fmt.Errorf("ref must start with #/components/: %s", ref)
	}

	if parts[1] != "schemas" {
		return nil, fmt.Errorf("only schema refs supported, got: %s", parts[1])
	}

	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid schema ref format: %s", ref)
	}

	name := parts[2]
	if comps.Schemas == nil {
		return nil, fmt.Errorf("no components.schemas defined")
	}

	sch, ok := comps.Schemas[name]
	if !ok {
		return nil, fmt.Errorf("schema not found: %s", name)
	}

	return sch, nil
}

// ImportToStorage records an OpenAPI document's declared contracts under one
// project.
//
// The project is a parameter rather than something the store resolves for
// itself. Importing a client's spec is the act that decides which client those
// contracts belong to, and a store that quietly answered "whichever project the
// dashboard happens to be showing" would file a client's API under whichever
// other client was on screen — the quiet misattribution this codebase is
// otherwise strict about.
func (s *OpenAPISpec) ImportToStorage(projectID string, store interface {
	SaveBaselineWithSchema(projectID, method, path, samplePayload string, declared *types.JSONSchemaNode, source string) (*types.ContractBaseline, error)
}) error {
	contracts, err := s.ExtractContracts()
	if err != nil {
		return err
	}

	for _, c := range contracts {
		var schemaNode *types.JSONSchemaNode
		if c.ResponseSchema != nil {
			schemaNode = c.ResponseSchema
		} else if c.RequestSchema != nil {
			schemaNode = c.RequestSchema
		}

		if schemaNode == nil {
			continue
		}

		sample := generateSample(schemaNode)

		// The parsed schema goes to storage as it is, rather than being
		// regenerated there from the sample. The sample was generated *from* this
		// node, so inference over it can only recover less: it loses the
		// document's `required` list and the formats it declared, and replaces
		// both with what one example happened to contain.
		//
		// A spec is a declared contract, so these versions arrive already
		// vouched for — not by a person clicking, but by the document the API
		// owner published. They must not read as provisional, or the dashboard
		// would ask the user to confirm what they just told us.
		_, err := store.SaveBaselineWithSchema(projectID, c.Method, c.Path, sample, schemaNode, types.BaselineSourceSpec)
		if err != nil {
			return fmt.Errorf("save baseline for %s %s: %w", c.Method, c.Path, err)
		}
	}

	return nil
}

func generateSample(node *types.JSONSchemaNode) string {
	if node == nil {
		return "{}"
	}

	var buf strings.Builder
	writeSample(&buf, node)
	return buf.String()
}

func writeSample(buf *strings.Builder, node *types.JSONSchemaNode) {
	if node == nil {
		buf.WriteString("null")
		return
	}

	switch node.Type {
	case types.TypeString:
		if node.Format != "" {
			buf.WriteString(fmt.Sprintf("\"%s\"", exampleForFormat(node.Format)))
		} else {
			buf.WriteString("\"example\"")
		}
	case types.TypeInteger:
		buf.WriteString("42")
	case types.TypeNumber:
		buf.WriteString("3.14")
	case types.TypeBoolean:
		buf.WriteString("true")
	case types.TypeNull:
		buf.WriteString("null")
	case types.TypeArray:
		buf.WriteString("[")
		if node.ItemSchema != nil {
			writeSample(buf, node.ItemSchema)
		}
		buf.WriteString("]")
	case types.TypeObject:
		buf.WriteString("{")
		first := true
		for k, v := range node.Properties {
			if !first {
				buf.WriteString(",")
			}
			first = false
			buf.WriteString(fmt.Sprintf("\"%s\":", k))
			writeSample(buf, v)
		}
		buf.WriteString("}")
	default:
		buf.WriteString("null")
	}
}

func exampleForFormat(f string) string {
	switch f {
	case "date":
		return "2024-01-15"
	case "date-time":
		return "2024-01-15T10:30:00Z"
	case "uuid":
		return "550e8400-e29b-41d4-a716-446655440000"
	case "email":
		return "user@example.com"
	case "uri":
		return "https://example.com"
	default:
		return "example"
	}
}
