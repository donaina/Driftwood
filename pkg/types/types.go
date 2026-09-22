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
	Format       string                     `json:"format,omitempty"` // date, date-time, uuid, email
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
	KindFormatChange      DiffKind = "FORMAT_CHANGE"
	// KindStatusCodeChange fits the naming of the others least: it is not a
	// difference between two payloads but the absence of a payload to compare.
	// It exists because the alternative — diffing an error body against a
	// baseline captured from a success — reports every field of the real
	// response as REMOVED_FIELD, burying the one fact worth knowing.
	KindStatusCodeChange DiffKind = "STATUS_CODE_CHANGE"
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

// Alert represents a contract alert with optional AI explanation.
//
// An alert is one endpoint breaking, not one field changing: the deltas that
// describe the break hang off Diff, and the explanation attached to the alert
// is the sidecar's reading of all of them together. Two earlier types sketched
// this the other way round — an alert per delta, with the explanation as a bare
// *string — and never acquired a caller. They were deleted rather than left
// beside the real shape, because a second, wrong description of the same
// concept is how the next reader picks the wrong one.
type Alert struct {
	TrafficID      string                 `json:"traffic_id"`
	Endpoint       string                 `json:"endpoint"`
	ContractStatus string                 `json:"contract_status"`
	Diff           *ContractDiff          `json:"diff,omitempty"`
	AIExplanation  map[string]interface{} `json:"ai_explanation,omitempty"`
}

// ContractDiff contains all diff deltas for a response
type ContractDiff struct {
	HasBreakingChanges bool        `json:"has_breaking_changes"`
	HasWarnings        bool        `json:"has_warnings"`
	Deltas             []DiffDelta `json:"deltas"`
}

// Where a contract version came from. The three provenances do not carry the
// same authority, and conflating them is what let Driftwood bless a response it
// had merely happened to see first as though a human had vouched for it.
const (
	// BaselineSourceAuto is a version captured from live traffic. It is the only
	// source that is a guess: nothing has told Driftwood this response was
	// correct, so drift that predates Driftwood would be measured against it and
	// found absent.
	BaselineSourceAuto = "auto"
	// BaselineSourceManual is a shape a human accepted, either by promoting a
	// payload or by confirming an auto-captured version.
	BaselineSourceManual = "manual"
	// BaselineSourceSpec is a contract declared by an imported OpenAPI document.
	// It is the only source with an authority outside this process.
	BaselineSourceSpec = "openapi"
)

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
	// Source is one of the BaselineSource* values, or "" for a baseline written
	// before this field existed. See IsProvisional.
	Source string `json:"source"`
}

// IsProvisional reports whether this version is an unconfirmed guess rather
// than a contract someone stood behind.
//
// An empty Source counts as provisional. Versions predating the field were, in
// the overwhelming majority, auto-captured from live traffic — the explicit
// routes existed but had no UI, so almost nothing reached them. Reading "" as
// confirmed would assert a human vouched for every one of those baselines, which
// is precisely the claim we cannot make.
func (c *ContractBaseline) IsProvisional() bool {
	return c.Source == "" || c.Source == BaselineSourceAuto
}

// Observation is one sighting of an endpoint — a single request Driftwood
// sniffed and checked against that endpoint's contract.
//
// Observations and Versions are deliberately separate. A Version is a shape
// somebody accepted; an Observation is the endpoint being seen again. Driftwood
// watches for a contract to *drift*, which is a claim about behaviour over
// time, so it needs the second series: with only Versions there is nothing to
// say an endpoint has been stable for a week, and nothing to draw a trend from.
type Observation struct {
	Timestamp      time.Time `json:"timestamp"`
	StatusCode     int       `json:"status_code"`
	DurationMs     int64     `json:"duration_ms"`
	ContractStatus string    `json:"contract_status"`
}

// EndpointHistory stores versioned history for a single endpoint
type EndpointHistory struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Versions      []*ContractBaseline `json:"versions"`
	LockedVersion int                 `json:"locked_version"` // 0 = latest, else specific version
	// ObservationCount is the true total number of sightings, including those
	// aged out of Observations. It used to be incremented by SaveBaseline, so
	// it counted baseline saves — a number that only moved when a human clicked
	// a button, under a name that promised a measurement of traffic.
	ObservationCount int64 `json:"observation_count"`
	// Observations is a bounded window of recent sightings, oldest first.
	// Bounded because this grows once per proxied request.
	Observations []Observation `json:"observations"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// CapturedTraffic holds full metadata for an intercepted HTTP transaction
type CapturedTraffic struct {
	ID string `json:"id"`
	// ProjectID is the project this request was sniffed under. It is set by the
	// store when the request is recorded, from the project the caller named, so
	// a record always says which context produced it rather than leaving a
	// reader to infer it from which list it happens to be in.
	ProjectID       string            `json:"project_id,omitempty"`
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
	DevMockMode      bool   `json:"dev_mock_mode"` // enable mock fallback for dev
}

// Project is one namespaced context: a name, a backend to sniff, and the
// endpoints recorded under it.
//
// TargetAllowPrivate persists the SSRF decision alongside the target it applies
// to, rather than re-deriving it on every start. That is sound because a private
// target can only be set from loopback in the first place: the field records an
// authorisation the operator already gave, it does not grant a new one. Without
// it, a restart would have to either re-block a target the operator configured
// deliberately — breaking every local-development install — or accept private
// targets unconditionally, which is the bug that put the blocklist in a
// constructor nobody called.
type Project struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	TargetURL          string    `json:"target_url"`
	TargetAllowPrivate bool      `json:"target_allow_private"`
	CreatedAt          time.Time `json:"created_at"`
}

// EventMessage represents real-time updates broadcast to WebSocket/SSE clients
type EventMessage struct {
	Type      string      `json:"type"` // "traffic", "alert", "baseline_updated", "config_updated"
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Where an alert can be delivered.
//
// Four channels, and the list is closed: each one is a POST to a URL the
// operator supplies, and they differ only in the shape of the JSON body. That is
// the whole reason they are one mechanism with a name rather than four
// mechanisms — everything except Render is shared.
//
// Email is deliberately not here. It is a different transport with different
// configuration (a host, a port, credentials, a from-address), net/smtp is
// frozen upstream, and the dashboard has no form for any of it. A channel that
// cannot be configured is a card whose Save button lies.
const (
	WebhookSlack   = "slack"
	WebhookTeams   = "teams"
	WebhookDiscord = "discord"
	WebhookGeneric = "generic"
)

// WebhookKinds is every kind, in a fixed order. It is a slice rather than a map
// because two callers need to iterate it deterministically: the store, so an
// unchanged document writes byte-identical bytes, and the dashboard, so the
// cards do not reorder themselves between loads.
var WebhookKinds = []string{WebhookSlack, WebhookTeams, WebhookDiscord, WebhookGeneric}

// IsWebhookKind reports whether kind names a channel this build can deliver to.
// A request carrying anything else is refused rather than stored: a config for a
// kind nothing renders is a setting with no effect, which is the shape of lie
// this feature exists to remove.
func IsWebhookKind(kind string) bool {
	for _, k := range WebhookKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// WebhookConfig is one project's delivery settings for one channel.
//
// Secret is stored in the clear, in the 0600 store inside a 0700 directory, and
// it is never returned by any route — the API answers has_secret instead. That
// is a deliberate trade rather than an oversight: the alternative is not storing
// it, and a signing secret the operator cannot retrieve is one they cannot use.
// What it buys is that a dashboard screenshot, a browser cache or a logged
// response body cannot leak it, because none of them ever contain it.
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Kind    string `json:"kind"`
	URL     string `json:"url"`
	// Secret signs the generic body; see the deliverer's signature headers. Empty
	// for the three vendor kinds, which authenticate by the URL's own token.
	Secret    string    `json:"secret,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
