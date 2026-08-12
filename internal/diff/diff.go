package diff

import (
	"fmt"

	"github.com/callmidavid/apidiff/internal/schema"
	"github.com/callmidavid/apidiff/pkg/types"
)

// CompareJSON compares raw baseline JSON with current raw JSON
func CompareJSON(baselineJSON, currentJSON string) (*types.ContractDiff, error) {
	baseNode, err := schema.InferFromJSON(baselineJSON)
	if err != nil {
		return nil, fmt.Errorf("baseline JSON invalid: %w", err)
	}

	currNode, err := schema.InferFromJSON(currentJSON)
	if err != nil {
		return nil, fmt.Errorf("current JSON invalid: %w", err)
	}

	return CompareSchemas(baseNode, currNode), nil
}

// CompareSchemas performs a recursive structural diff between baseline schema and current schema
func CompareSchemas(baseline, current *types.JSONSchemaNode) *types.ContractDiff {
	diffResult := &types.ContractDiff{
		Deltas: make([]types.DiffDelta, 0),
	}

	compareRecursive(baseline, current, "$", diffResult)

	for _, d := range diffResult.Deltas {
		if d.Severity == types.SeverityBreaking {
			diffResult.HasBreakingChanges = true
		}
		if d.Severity == types.SeverityWarning {
			diffResult.HasWarnings = true
		}
	}

	return diffResult
}

func compareRecursive(base, curr *types.JSONSchemaNode, path string, diff *types.ContractDiff) {
	if base == nil && curr == nil {
		return
	}

	if base == nil {
		// New node at path
		diff.Deltas = append(diff.Deltas, types.DiffDelta{
			JSONPath: path,
			Kind:     types.KindAddedField,
			Severity: types.SeverityInfo,
			Message:  fmt.Sprintf("New field added: %s (type %s)", path, curr.Type),
			Expected: "<non-existent>",
			Actual:   string(curr.Type),
		})
		return
	}

	if curr == nil {
		// Removed node at path
		diff.Deltas = append(diff.Deltas, types.DiffDelta{
			JSONPath: path,
			Kind:     types.KindRemovedField,
			Severity: types.SeverityBreaking,
			Message:  fmt.Sprintf("🚨 Removed field: '%s' expected type '%s' but field is missing", path, base.Type),
			Expected: string(base.Type),
			Actual:   "<missing>",
		})
		return
	}

	// Nullability check
	if curr.Type == types.TypeNull && base.Type != types.TypeNull {
		if !base.Nullable {
			diff.Deltas = append(diff.Deltas, types.DiffDelta{
				JSONPath: path,
				Kind:     types.KindNullabilityChange,
				Severity: types.SeverityBreaking,
				Message:  fmt.Sprintf("🚨 Nullability contract violation: '%s' expected non-null '%s' but received null", path, base.Type),
				Expected: string(base.Type),
				Actual:   "null",
			})
			return
		}
	}

	// Type comparison
	if !isCompatibleType(base.Type, curr.Type) {
		diff.Deltas = append(diff.Deltas, types.DiffDelta{
			JSONPath: path,
			Kind:     types.KindTypeMismatch,
			Severity: types.SeverityBreaking,
			Message:  fmt.Sprintf("🚨 TYPE MISMATCH at '%s': expected '%s', got '%s'", path, base.Type, curr.Type),
			Expected: string(base.Type),
			Actual:   string(curr.Type),
		})
		return
	}

	// Recurse for Objects
	if base.Type == types.TypeObject && curr.Type == types.TypeObject {
		// Check for missing keys or modified keys in current
		for key, baseProp := range base.Properties {
			if IsNoiseKey(key) {
				continue
			}
			childPath := path + "." + key
			currProp, exists := curr.Properties[key]
			if !exists {
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: childPath,
					Kind:     types.KindRemovedField,
					Severity: types.SeverityBreaking,
					Message:  fmt.Sprintf("🚨 Missing property '%s' (expected type '%s')", childPath, baseProp.Type),
					Expected: string(baseProp.Type),
					Actual:   "<missing>",
				})
			} else {
				compareRecursive(baseProp, currProp, childPath, diff)
			}
		}

		// Check for newly added keys in current
		for key, currProp := range curr.Properties {
			if IsNoiseKey(key) {
				continue
			}
			childPath := path + "." + key
			if _, exists := base.Properties[key]; !exists {
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: childPath,
					Kind:     types.KindAddedField,
					Severity: types.SeverityInfo,
					Message:  fmt.Sprintf("✨ Added new field '%s' of type '%s'", childPath, currProp.Type),
					Expected: "<absent>",
					Actual:   string(currProp.Type),
				})
			}
		}
	}

	// Recurse for Arrays
	if base.Type == types.TypeArray && curr.Type == types.TypeArray {
		if base.ItemSchema != nil && curr.ItemSchema != nil {
			compareRecursive(base.ItemSchema, curr.ItemSchema, path+"[*]", diff)
		}
	}
}

// isCompatibleType returns true if types match or are compatible (e.g. integer can match number)
func isCompatibleType(base, curr types.JSONNodeType) bool {
	if base == curr {
		return true
	}
	if base == types.TypeUnknown || curr == types.TypeUnknown {
		return true
	}
	// Integer to float can be compatible in loose JSON contexts, but integer to string is NOT!
	if base == types.TypeInteger && curr == types.TypeNumber {
		return true
	}
	return false
}
