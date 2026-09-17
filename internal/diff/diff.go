package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/donaina/driftwood/internal/schema"
	"github.com/donaina/driftwood/pkg/types"
)

// CompareJSON compares raw baseline JSON with current raw JSON.
//
// Both sides are inferred from their payloads, so the baseline is described by
// what it happened to contain. Prefer CompareJSONWithSchema when the baseline's
// own schema is available: inference cannot represent an optional field, so
// anything only a stored schema knows — a spec's `required` list above all — is
// thrown away here.
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

// CompareJSONWithSchema compares a stored baseline schema against a live
// response, inferring only the response.
//
// The baseline is taken as given rather than re-inferred from its sample,
// because the stored schema is the only thing that can carry what a spec
// declared: an OpenAPI import records the document's `required` list, and
// re-inferring the sample replaces it with "every key present is required",
// which is a different and stronger claim than the document made.
func CompareJSONWithSchema(baseline *types.JSONSchemaNode, currentJSON string) (*types.ContractDiff, error) {
	if baseline == nil {
		return nil, fmt.Errorf("baseline schema is nil")
	}

	currNode, err := schema.InferFromJSON(currentJSON)
	if err != nil {
		return nil, fmt.Errorf("current JSON invalid: %w", err)
	}

	return CompareSchemas(baseline, currNode), nil
}

// isRequiredProperty reports whether the baseline object declared key as a
// required property.
//
// RequiredKeys lives on the object node, not on the property node. Where it
// comes from decides what an answer means:
//
//   - A schema inferred from a payload lists every key it saw, because one
//     sample cannot distinguish an optional field from a mandatory one. Those
//     baselines keep the old behaviour: a removed field is breaking.
//   - A schema imported from a spec lists the document's `required` entries,
//     which is the only place a real optionality claim can come from today.
//   - An absent or empty list means nothing is known to be required, so a
//     removal is not treated as a broken promise.
//
// The gap this leaves is documented at schema.Infer's object case: Driftwood
// could learn optionality by tracking which keys appear in every observation
// rather than some, and it does not yet.
func isRequiredProperty(object *types.JSONSchemaNode, key string) bool {
	for _, k := range object.RequiredKeys {
		if k == key {
			return true
		}
	}
	return false
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

	// Format check. Format is the recognised shape of a string value — a date, a
	// UUID, an email — either inferred from a payload or declared by a spec, and
	// it was recorded without ever being read, so a date that became a UUID was
	// invisible. Nothing else here sees it: the JSON type is `string` on both
	// sides, so this is the only place the change can surface.
	if base.Type == types.TypeString && curr.Type == types.TypeString && base.Format != curr.Format {
		delta := types.DiffDelta{
			JSONPath: path,
			Kind:     types.KindFormatChange,
		}
		switch {
		case base.Format != "" && curr.Format != "":
			// A date that became a UUID is a break: every client parsing the old
			// shape is affected, and the change is deliberate rather than
			// incidental.
			delta.Severity = types.SeverityBreaking
			delta.Message = "format changed"
			delta.Expected = base.Format
			delta.Actual = curr.Format
		case base.Format != "":
			// A known format that no longer matches is weaker evidence. The value
			// may be a placeholder ("", "N/A") rather than a different kind of
			// value, so this is reported, not alerted.
			delta.Severity = types.SeverityWarning
			delta.Message = "format no longer recognized"
			delta.Expected = base.Format
			delta.Actual = "<absent>"
		default:
			// Gaining a format narrows the shape rather than changing it.
			delta.Severity = types.SeverityInfo
			delta.Message = "format recognized"
			delta.Expected = "<absent>"
			delta.Actual = curr.Format
		}
		diff.Deltas = append(diff.Deltas, delta)
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
				// Removed field. Whether that is breaking depends on whether the
				// baseline promised the field or merely showed it once.
				//
				// RequiredKeys was written and never read, so every removal was
				// BREAKING whether or not the field was ever guaranteed. For an
				// OpenAPI-imported contract that is simply wrong: the spec states
				// which properties are required, and a property outside that list
				// was never part of the promise being broken.
				//
				// For an inferred baseline the two are the same thing, because
				// inference marks every key it has seen as required — see
				// InferObject, which documents why one sample cannot tell an
				// optional field from a mandatory one.
				severity := types.SeverityBreaking
				message := "removed"
				if !isRequiredProperty(base, key) {
					severity = types.SeverityWarning
					message = "removed, but the baseline did not require it"
				}
				diff.Deltas = append(diff.Deltas, types.DiffDelta{
					JSONPath: childPath,
					Kind:     types.KindRemovedField,
					Severity: severity,
					Message:  message,
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
