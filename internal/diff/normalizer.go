package diff

import (
	"regexp"
	"strings"

	"github.com/callmidavid/apidiff/pkg/types"
)

// Dynamic noise key patterns (e.g. request IDs, trace IDs, timestamps)
var defaultNoisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(request_?id|req_?id|trace_?id|span_?id|correlation_?id|nonce|etag)$`),
	regexp.MustCompile(`(?i)^(timestamp|time|created_?at|updated_?at|server_?time)$`),
}

// NormalizeConfig holds normalization options
type NormalizeConfig struct {
	StripNoiseKeys bool
	SortArrayItems bool
}

// DefaultNormalizeConfig returns standard recommended normalization settings
func DefaultNormalizeConfig() NormalizeConfig {
	return NormalizeConfig{
		StripNoiseKeys: true,
		SortArrayItems: true,
	}
}

// IsNoiseKey checks if a property key matches dynamic noise patterns (timestamps, request IDs, trace IDs)
func IsNoiseKey(key string) bool {
	for _, pattern := range defaultNoisePatterns {
		if pattern.MatchString(key) {
			return true
		}
	}
	return false
}

// ClassifyDeltas separates diff deltas into breaking vs additive (non-breaking) categories
func ClassifyDeltas(deltas []types.DiffDelta) (breaking []types.DiffDelta, additive []types.DiffDelta) {
	for _, delta := range deltas {
		switch delta.Severity {
		case types.SeverityBreaking:
			breaking = append(breaking, delta)
		case types.SeverityInfo, types.SeverityWarning:
			additive = append(additive, delta)
		}
	}
	return breaking, additive
}

// CleanPath extracts the property basename from a JSONPath string
func CleanPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}
