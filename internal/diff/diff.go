package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
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

func isValidJSONIdentifier(key string) bool {
	if key == "" {
		return false
	}
	// First char: letter, underscore
	first := key[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}
	// Rest: alphanumeric, underscore
	for i := 1; i < len(key); i++ {
		c := key[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func formatPath(path, key string) string {
	if path == "$" {
		if isValidJSONIdentifier(key) {
			return "$." + key
		}
		return "$['" + escapeSingleQuote(key) + "']"
	}
	if isValidJSONIdentifier(key) {
		return path + "." + key
	}
	return path + "['" + escapeSingleQuote(key) + "']"
}

func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
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
			Message:  "added",
			Expected: "<absent>",
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
			Message:  "removed",
			Expected: string(base.Type),
			Actual:   "<absent>",
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
				Message:  "nullability violation",
				Expected: string(base.Type),
				Actual:   "null",
			})
			return // Early return only for actual violation
		}
		// If base.Nullable == true, fall through to type checking
	}

	// Type comparison
	if !isCompatibleType(base.Type, curr.Type) {
		// Determine severity: type widen (integer<->number) = WARNING, null->typed = INFO (refinement), else BREAKING
		severity := types.SeverityBreaking
		if (base.Type == types.TypeInteger && curr.Type == types.TypeNumber) ||
			(base.Type == types.TypeNumber && curr.Type == types.TypeInteger) {
			severity = types.SeverityWarning
		}
		if base.Type == types.TypeNull && curr.Type != types.TypeNull {
			severity = types.SeverityInfo
		}

		diff.Deltas = append(diff.Deltas, types.DiffDelta{
			JSONPath: path,
			Kind:     types.KindTypeMismatch,
			Severity: severity,
			Message:  "type mismatch",
			Expected: string(base.Type),
			Actual:   string(curr.Type),
		})
		return // After type mismatch, stop recursing
	}

	// Recurse for Objects - deterministic ordering via sorted keys
	if base.Type == types.TypeObject && curr.Type == types.TypeObject {
		// Collect all keys, sort for deterministic ordering
		allKeys := make(map[string]bool)
		for k := range base.Properties {
			allKeys[k] = true
		}
		for k := range curr.Properties {
			allKeys[k] = true
		}

		keys := make([]string, 0, len(allKeys))
		for k := range allKeys {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if IsNoiseKey(key) {
				continue
			}
			childPath := formatPath(path, key)
			baseProp, inBase := base.Properties[key]
			currProp, inCurr := curr.Properties[key]

			if inBase && !inCurr {
				// Removed field
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: childPath,
					Kind:     types.KindRemovedField,
					Severity: types.SeverityBreaking,
					Message:  "removed",
					Expected: string(baseProp.Type),
					Actual:   "<absent>",
				})
			} else if !inBase && inCurr {
				// Added field
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: childPath,
					Kind:     types.KindAddedField,
					Severity: types.SeverityInfo,
					Message:  "added",
					Expected: "<absent>",
					Actual:   string(currProp.Type),
				})
			} else if inBase && inCurr {
				// Modified field - recurse
				compareRecursive(baseProp, currProp, childPath, diff)
			}
		}
	}

	// Recurse for Arrays
	if base.Type == types.TypeArray && curr.Type == types.TypeArray {
		if base.ItemSchema != nil && curr.ItemSchema != nil {
			// Check for empty-array unknown item case
			isEmptyArrayUnknown := base.ItemSchema.Type == types.TypeUnknown &&
				base.ItemSchema.SampleValue == nil &&
				curr.ItemSchema.Type != types.TypeUnknown

			if isEmptyArrayUnknown {
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: path + "[*]",
					Kind:     types.KindArrayTypeMismatch,
					Severity: types.SeverityBreaking,
					Message:  "array item type mismatch",
					Expected: "unknown (empty array)",
					Actual:   string(curr.ItemSchema.Type),
				})
				return
			}

			compareRecursive(base.ItemSchema, curr.ItemSchema, path+"[*]", diff)
		}
	}
}

// isCompatibleType returns true if types match or are compatible (e.g. integer↔number both directions)
func isCompatibleType(base, curr types.JSONNodeType) bool {
	if base == curr {
		return true
	}
	if base == types.TypeUnknown || curr == types.TypeUnknown {
		return true
	}
	// Integer to float can be compatible in loose JSON contexts (BOTH directions)
	if (base == types.TypeInteger && curr == types.TypeNumber) ||
		(base == types.TypeNumber && curr == types.TypeInteger) {
		return true
	}
	return false
}