package capture

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/donaina/driftwood/pkg/types"
)

// Common patterns for sensitive data
var (
	// Credit card numbers (basic patterns)
	ccPattern = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
	// SSN pattern
	ssnPattern = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	// API keys / tokens (common prefixes)
	apiKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|secret[_-]?key|auth[_-]?token)\s*[:=]\s*[\w-]+`)
	// JWT tokens
	jwtPattern = regexp.MustCompile(`eyJ[\w-]+\.[\w-]+\.[\w-]+`)
)

// SanitizeTraffic removes sensitive header tokens (Bearer, Authorization) before storage
func SanitizeTraffic(traffic *types.CapturedTraffic) {
	if traffic == nil {
		return
	}

	// Sanitize headers
	sanitizeHeaders(traffic.RequestHeaders)
	sanitizeHeaders(traffic.ResponseHeaders)

	// Sanitize bodies
	traffic.RequestBody = SanitizeBody(traffic.RequestBody)
	traffic.ResponseBody = SanitizeBody(traffic.ResponseBody)
}

func sanitizeHeaders(headers map[string]string) {
	sensitive := map[string]bool{
		"authorization":       true,
		"cookie":              true,
		"set-cookie":          true,
		"x-api-key":           true,
		"x-auth-token":        true,
		"proxy-authorization": true,
	}
	for k := range headers {
		if sensitive[strings.ToLower(k)] {
			headers[k] = "[REDACTED]"
		}
	}
}

// SanitizeBody removes sensitive patterns from JSON/request bodies
func SanitizeBody(body string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}

	// Try to parse as JSON and sanitize object fields
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err == nil {
		sanitizeJSON(obj)
		if b, err := json.Marshal(obj); err == nil {
			return string(b)
		}
	}

	// Fallback: regex-based sanitization for non-JSON
	result := ccPattern.ReplaceAllString(body, "[REDACTED_CC]")
	result = ssnPattern.ReplaceAllString(result, "[REDACTED_SSN]")
	result = apiKeyPattern.ReplaceAllString(result, "[REDACTED_API_KEY]")
	result = jwtPattern.ReplaceAllString(result, "[REDACTED_JWT]")
	return result
}

func sanitizeJSON(obj map[string]interface{}) {
	sensitiveKeys := map[string]bool{
		"password":       true,
		"secret":         true,
		"token":          true,
		"apikey":         true,
		"api_key":        true,
		"access_token":   true,
		"refresh_token":  true,
		"authorization":  true,
		"credit_card":    true,
		"creditcard":     true,
		"cc_number":      true,
		"ssn":            true,
		"social_security": true,
	}

	for k, v := range obj {
		lower := strings.ToLower(k)
		if sensitiveKeys[lower] || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
			obj[k] = "[REDACTED]"
			continue
		}
		// Recurse into nested objects
		if nested, ok := v.(map[string]interface{}); ok {
			sanitizeJSON(nested)
		}
		// Recurse into arrays of objects
		if arr, ok := v.([]interface{}); ok {
			for _, elem := range arr {
				if nested, ok := elem.(map[string]interface{}); ok {
					sanitizeJSON(nested)
				}
			}
		}
	}
}