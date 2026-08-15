package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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
	OperationID string                 `json:"operationId"`
	Summary     string                 `json:"summary"`
	Description string                 `json:"description"`
	Parameters  []Parameter            `json:"parameters"`
	RequestBody *RequestBody           `json:"requestBody"`
	Responses   map[string]Response    `json:"responses"`
}

type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      *Schema `json:"schema"`
}

type RequestBody struct {
	Description string              `json:"description"`
	Content     map[string]MediaType `json:"content"`
	Required    bool                `json:"required"`
}

type Response struct {
	Description string              `json:"description"`
	Content     map[string]MediaType `json:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema"`
}

type Schema struct {
	Type                 string              `json:"type"`
	Format               string              `json:"format"`
	Title                string              `json:"title"`
	Description          string              `json:"description"`
	Enum                 []interface{}       `json:"enum"`
	Const                interface{}         `json:"const"`
	Default              interface{}         `json:"default"`
	MultipleOf           *float64            `json:"multipleOf"`
	Maximum              *float64            `json:"maximum"`
	ExclusiveMaximum     interface{}         `json:"exclusiveMaximum"`
	Minimum              *float64            `json:"minimum"`
	ExclusiveMinimum     interface{}         `json:"exclusiveMinimum"`
	MaxLength            *int                `json:"maxLength"`
	MinLength            *int                `json:"minLength"`
	Pattern              string              `json:"pattern"`
	MaxItems             *int                `json:"maxItems"`
	MinItems             *int                `json:"minItems"`
	UniqueItems          bool                `json:"uniqueItems"`
	MaxProperties        *int                `json:"maxProperties"`
	MinProperties        *int                `json:"minProperties"`
	Required             []string            `json:"required"`
	Properties           map[string]*Schema  `json:"properties"`
	AdditionalProperties interface{}         `json:"additionalProperties"`
	Items                *Schema             `json:"items"`
	AllOf                []*Schema           `json:"allOf"`
	AnyOf                []*Schema           `json:"anyOf"`
	OneOf                []*Schema           `json:"oneOf"`
	Not                  *Schema             `json:"not"`
	Ref                  string              `json:"$ref"`
}

type Components struct {
	Schemas         map[string]*Schema `json:"schemas"`
	Responses       map[string]Response `json:"responses"`
	Parameters      map[string]Parameter `json:"parameters"`
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

func LoadFromURL(url string) (*OpenAPISpec, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
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
		Nullable:    false,
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

func (s *OpenAPISpec) ImportToStorage(store interface {
	SaveBaseline(method, path, samplePayload string) (*types.ContractBaseline, error)
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

		_, err := store.SaveBaseline(c.Method, c.Path, sample)
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