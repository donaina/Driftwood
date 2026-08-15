package schema

import (
	"encoding/json"
	"math"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

var (
	dateRegex     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dateTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?$`)
	uuidRegex     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// InferFromJSON parses a JSON string and builds its JSONSchemaNode tree
func InferFromJSON(jsonStr string) (*types.JSONSchemaNode, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return &types.JSONSchemaNode{Type: types.TypeNull}, nil
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, err
	}

	return Infer(raw), nil
}

// Infer builds a JSONSchemaNode tree from an unmarshaled interface{}
func Infer(val interface{}) *types.JSONSchemaNode {
	if val == nil {
		return &types.JSONSchemaNode{
			Type:     types.TypeNull,
			Nullable: true,
		}
	}

	switch v := val.(type) {
	case bool:
		return &types.JSONSchemaNode{
			Type:        types.TypeBoolean,
			SampleValue: v,
		}
	case string:
		node := &types.JSONSchemaNode{
			Type:        types.TypeString,
			SampleValue: v,
		}
		node.Format = detectFormat(v)
		return node
	case float64:
		// Distinguish integer vs floating point number
		if math.Floor(v) == v && !math.IsInf(v, 0) {
			return &types.JSONSchemaNode{
				Type:        types.TypeInteger,
				SampleValue: int64(v),
			}
		}
		return &types.JSONSchemaNode{
			Type:        types.TypeNumber,
			SampleValue: v,
		}
	case int:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case int8:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case int16:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case int32:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case int64:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: v}
	case uint:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case uint8:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case uint16:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case uint32:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case uint64:
		return &types.JSONSchemaNode{Type: types.TypeInteger, SampleValue: int64(v)}
	case map[string]interface{}:
		props := make(map[string]*types.JSONSchemaNode)
		reqKeys := make([]string, 0, len(v))

		for k, childVal := range v {
			reqKeys = append(reqKeys, k)
			props[k] = Infer(childVal)
		}
		sort.Strings(reqKeys)

		return &types.JSONSchemaNode{
			Type:         types.TypeObject,
			Properties:   props,
			RequiredKeys: reqKeys,
		}
	case []interface{}:
		node := &types.JSONSchemaNode{
			Type: types.TypeArray,
		}

		if len(v) == 0 {
			node.ItemSchema = &types.JSONSchemaNode{Type: types.TypeUnknown}
		} else {
			// Merge schema across array items to capture optional/nullable fields
			var itemSchema *types.JSONSchemaNode
			for _, elem := range v {
				elemNode := Infer(elem)
				if itemSchema == nil {
					itemSchema = elemNode
				} else {
					itemSchema = mergeNodes(itemSchema, elemNode)
				}
			}
			node.ItemSchema = itemSchema
		}
		return node
	default:
		return &types.JSONSchemaNode{
			Type: types.TypeUnknown,
		}
	}
}

func detectFormat(s string) string {
	// Check date-time first (more specific)
	if dateTimeRegex.MatchString(s) {
		if _, err := time.Parse(time.RFC3339, s); err == nil {
			return "date-time"
		}
	}
	// Check date
	if dateRegex.MatchString(s) {
		if _, err := time.Parse("2006-01-02", s); err == nil {
			return "date"
		}
	}
	// Check UUID
	if uuidRegex.MatchString(strings.ToLower(s)) {
		return "uuid"
	}
	// Check email
	if emailRegex.MatchString(s) {
		if _, err := mail.ParseAddress(s); err == nil {
			return "email"
		}
	}
	return ""
}

// mergeNodes merges two schema nodes (useful for unifying array element schemas)
// Returns a NEW node - does not mutate inputs (fixes aliasing bug #31)
func mergeNodes(a, b *types.JSONSchemaNode) *types.JSONSchemaNode {
	if a == nil {
		return deepCopy(b)
	}
	if b == nil {
		return deepCopy(a)
	}

	if a.Type == types.TypeNull {
		res := deepCopy(b)
		res.Nullable = true
		return res
	}
	if b.Type == types.TypeNull {
		res := deepCopy(a)
		res.Nullable = true
		return res
	}

	// Standard type match
	if a.Type == b.Type {
		if a.Type == types.TypeObject {
			// Deep copy base properties, then merge
			mergedProps := make(map[string]*types.JSONSchemaNode)
			allKeys := make(map[string]bool)

			for k := range a.Properties {
				allKeys[k] = true
			}
			for k := range b.Properties {
				allKeys[k] = true
			}

			reqKeys := make([]string, 0)
			for k := range allKeys {
				propA, inA := a.Properties[k]
				propB, inB := b.Properties[k]

				if inA && inB {
					mergedProps[k] = mergeNodes(propA, propB)
					reqKeys = append(reqKeys, k)
				} else if inA {
					copied := deepCopy(propA)
					copied.Nullable = true
					mergedProps[k] = copied
				} else {
					copied := deepCopy(propB)
					copied.Nullable = true
					mergedProps[k] = copied
				}
			}
			sort.Strings(reqKeys)

			return &types.JSONSchemaNode{
				Type:         types.TypeObject,
				Properties:   mergedProps,
				RequiredKeys: reqKeys,
				Nullable:     a.Nullable || b.Nullable,
			}
		}

		if a.Type == types.TypeArray {
			return &types.JSONSchemaNode{
				Type:       types.TypeArray,
				ItemSchema: mergeNodes(a.ItemSchema, b.ItemSchema),
				Nullable:   a.Nullable || b.Nullable,
			}
		}

		// Primitive type match - create new node
		return &types.JSONSchemaNode{
			Type:        a.Type,
			Nullable:    a.Nullable || b.Nullable,
			SampleValue: a.SampleValue,
			Format:      a.Format,
		}
	}

	// Type mismatch (e.g. integer vs float -> float)
	if (a.Type == types.TypeInteger && b.Type == types.TypeNumber) ||
		(a.Type == types.TypeNumber && b.Type == types.TypeInteger) {
		return &types.JSONSchemaNode{
			Type:     types.TypeNumber,
			Nullable: a.Nullable || b.Nullable,
		}
	}

	// General union / mixed type
	return &types.JSONSchemaNode{
		Type:     types.TypeUnknown,
		Nullable: true,
	}
}

// deepCopy creates a deep copy of a JSONSchemaNode (no pointer aliasing)
func deepCopy(node *types.JSONSchemaNode) *types.JSONSchemaNode {
	if node == nil {
		return nil
	}
	copy := *node
	if node.Properties != nil {
		copy.Properties = make(map[string]*types.JSONSchemaNode, len(node.Properties))
		for k, v := range node.Properties {
			copy.Properties[k] = deepCopy(v)
		}
	}
	if node.ItemSchema != nil {
		copy.ItemSchema = deepCopy(node.ItemSchema)
	}
	return &copy
}