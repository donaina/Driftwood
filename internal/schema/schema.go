package schema

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/callmidavid/apidiff/pkg/types"
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
		return &types.JSONSchemaNode{
			Type:        types.TypeString,
			SampleValue: v,
		}
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

// mergeNodes merges two schema nodes (useful for unifying array element schemas)
func mergeNodes(a, b *types.JSONSchemaNode) *types.JSONSchemaNode {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	if a.Type == types.TypeNull {
		res := *b
		res.Nullable = true
		return &res
	}
	if b.Type == types.TypeNull {
		res := *a
		res.Nullable = true
		return &res
	}

	// Standard type match
	if a.Type == b.Type {
		if a.Type == types.TypeObject {
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
					propA.Nullable = true
					mergedProps[k] = propA
				} else {
					propB.Nullable = true
					mergedProps[k] = propB
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

		return &types.JSONSchemaNode{
			Type:        a.Type,
			Nullable:    a.Nullable || b.Nullable,
			SampleValue: a.SampleValue,
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
