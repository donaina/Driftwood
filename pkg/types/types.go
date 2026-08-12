package types

import "time"

// JSONNodeType represents the primitive or structural type in JSON Schema
type JSONNodeType string

const (
	TypeString  JSONNodeType = "string"
	TypeInteger JSONNodeType = "integer"
	TypeNumber  JSONNodeType = "number"
	TypeBoolean JSONNodeType = "boolean"
	TypeNull    JSONNodeType = "null"
	TypeArray   JSONNodeType = "array"
	TypeObject  JSONNodeType = "object"
	TypeUnknown JSONNodeType = "unknown"
)

// JSONSchemaNode represents an inferred schema node for a JSON structure
type JSONSchemaNode struct {
	Type         JSONNodeType               `json:"type"`
	Nullable     bool                       `json:"nullable,omitempty"`
	Properties   map[string]*JSONSchemaNode `json:"properties,omitempty"`
	ItemSchema   *JSONSchemaNode            `json:"item_schema,omitempty"`
	SampleValue  interface{}                `json:"sample_value,omitempty"`
	RequiredKeys []string                   `json:"required_keys,omitempty"`
}

// DiffSeverity classifies the impact of a schema diff
type DiffSeverity string

const (
	SeverityBreaking DiffSeverity = "BREAKING"
	SeverityWarning  DiffSeverity = "WARNING"
	SeverityInfo     DiffSeverity = "INFO"
)

// DiffKind identifies the specific nature of a field difference
type DiffKind string

const (
	KindTypeMismatch      DiffKind = "TYPE_MISMATCH"
	KindRemovedField      DiffKind = "REMOVED_FIELD"
	KindAddedField        DiffKind = "ADDED_FIELD"
	KindNullabilityChange DiffKind = "NULLABILITY_CHANGE"
	KindArrayTypeMismatch DiffKind = "ARRAY_ITEM_MISMATCH"
)

// DiffDelta details a single change between baseline and observed payload
type DiffDelta struct {
	JSONPath string       `json:"json_path"`
	Kind     DiffKind     `json:"kind"`
	Severity DiffSeverity `json:"severity"`
	Message  string       `json:"message"`
	Expected string       `json:"expected"`
	Actual   string       `json:"actual"`
}

// ContractDiff contains all diff deltas for a response
type ContractDiff struct {
	HasBreakingChanges bool        `json:"has_breaking_changes"`
	HasWarnings        bool        `json:"has_warnings"`
	Deltas             []DiffDelta `json:"deltas"`
}

// ContractBaseline represents a locked/cached contract for an endpoint
type ContractBaseline struct {
	ID            string          `json:"id"`
	Method        string          `json:"method"`
	Path          string          `json:"path"`
	Schema        *JSONSchemaNode `json:"schema"`
	SamplePayload string          `json:"sample_payload"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Version       int             `json:"version"`
	RequestCount  int64           `json:"request_count"`
}

// CapturedTraffic holds full metadata for an intercepted HTTP transaction
type CapturedTraffic struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	URL             string            `json:"url"`
	StatusCode      int               `json:"status_code"`
	DurationMs      int64             `json:"duration_ms"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	RequestBody     string            `json:"request_body,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	IsJSON          bool              `json:"is_json"`
	ContractStatus  string            `json:"contract_status"` // NO_BASELINE, MATCH, WARNING, BREAKING
	Diff            *ContractDiff     `json:"diff,omitempty"`
}

// ProxyConfig represents runtime configuration for Driftwood proxy
type ProxyConfig struct {
	TargetURL        string `json:"target_url"`
	ProxyPort        string `json:"proxy_port"`
	AutoSaveBaseline bool   `json:"auto_save_baseline"`
	InterceptJSON    bool   `json:"intercept_json"`
}

// EventMessage represents real-time updates broadcast to WebSocket/SSE clients
type EventMessage struct {
	Type      string      `json:"type"` // "traffic", "alert", "baseline_updated", "config_updated"
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}
