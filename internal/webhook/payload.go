package webhook

/* What a delivered alert says.

   The payload carries the deltas and nothing else. It is worth being exact about
   what that excludes and why, because both exclusions are load-bearing rather
   than incidental.

   No response body. A webhook leaves the building and lands somewhere Driftwood
   does not control — a Slack workspace, a vendor's queue, a channel with more
   people in it than the dashboard has readers — so the question is not "is this
   redacted well" but "what is structurally incapable of leaking". Response
   bodies are stored raw by design, because the dashboard's whole job is showing
   them; a payload that never contains one cannot leak a stored token by being
   rendered badly. What survives is the delta's JSONPath, which names the field
   and is already on screen in the Contract Alerts view.

   No AI explanation. It is free prose from a third-party model, it arrives up to
   eight seconds after the alert by design, and in the common case it does not
   exist yet when the first delivery is attempted. A payload that sometimes
   carries a paragraph and sometimes does not is worse than one that never does,
   and it is the only field in an alert whose content this product does not
   author. Carrying it properly means a second, follow-up delivery when it lands,
   which is a feature in its own right and is out of scope.

   Redaction still runs over what remains, as defence in depth rather than as the
   primary protection. See RedactPayload. */

import (
	"time"

	"github.com/donaina/driftwood/internal/capture"
	"github.com/donaina/driftwood/pkg/types"
)

// Event names. A test send announces itself as one to the human reading the
// channel, not only to the dashboard: this codebase has a scar about fabricating
// a contract that looks real — the deleted /api/users seed that made the first
// healthy response look BREAKING — and a test alert that is indistinguishable
// from a real one is that mistake wearing a different hat.
const (
	EventContractDrift = "contract_drift"
	EventTest          = "driftwood_test"
)

// ProjectRef identifies which project an alert came from, by name as well as id:
// a channel reader has never seen the id, and "acme" is what they configured.
type ProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Summary counts the deltas by severity, so a reader can see the shape of a
// break without parsing the list. It is computed, never reported from elsewhere.
type Summary struct {
	Breaking int `json:"breaking"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// Payload is the canonical body. Every kind renders from this, which is why the
// four channels are one mechanism with a kind rather than four mechanisms.
//
// The field names for endpoint, contract_status, traffic_id and deltas are taken
// from types.Alert's own JSON tags so the wire body and the dashboard's alert
// object agree by construction rather than by two people remembering the same
// spelling.
type Payload struct {
	Event      string            `json:"event"`
	Project    ProjectRef        `json:"project"`
	Endpoint   string            `json:"endpoint"`
	Status     string            `json:"contract_status"`
	TrafficID  string            `json:"traffic_id"`
	DetectedAt time.Time         `json:"detected_at"`
	Summary    Summary           `json:"summary"`
	Deltas     []types.DiffDelta `json:"deltas"`
}

// BuildPayload projects a stored alert into its wire form.
//
// Deltas is copied rather than aliased. The alert handed in shares its *Diff
// with the store's live alert — that sharing is deliberate and is what makes the
// copy cheap — and the store mutates that same struct when the sidecar answers.
// A payload holding a slice of the shared diff would be a payload whose bytes
// depend on when it was rendered.
func BuildPayload(event string, project ProjectRef, alert types.Alert, now time.Time) Payload {
	p := Payload{
		Event:      event,
		Project:    project,
		Endpoint:   alert.Endpoint,
		Status:     alert.ContractStatus,
		TrafficID:  alert.TrafficID,
		DetectedAt: now,
	}

	if alert.Diff != nil {
		p.Deltas = append([]types.DiffDelta(nil), alert.Diff.Deltas...)
		for _, d := range p.Deltas {
			switch d.Severity {
			case types.SeverityBreaking:
				p.Summary.Breaking++
			case types.SeverityWarning:
				p.Summary.Warning++
			default:
				p.Summary.Info++
			}
		}
	}
	p.Summary.Total = len(p.Deltas)

	return RedactPayload(p)
}

// RedactPayload sweeps the fields an operator's own data can reach.
//
// This is defence in depth, not the protection. BuildPayload's doc comment says
// what the real guarantee is: nothing describing response *content* enters the
// payload at all. What can enter is text the product does not author — the
// endpoint string, which carries a request path an API client wrote, and the
// delta messages, which are mostly product-authored but quote a JSON path.
//
// It is worth having because capture's redaction has a hole this closes. The
// value regexes there run only in SanitizeBody's non-JSON fallback; when a body
// parses as JSON, sanitizeJSON redacts *by key name* and never inspects a string
// value. So {"note": "the token eyJ... was returned"} passes SanitizeBody
// untouched. Anything that came from a request path can hold the same thing.
//
// Stated residual: no pattern in capture matches a bare email address, and
// adding a broad one would redact the demo seed's own alex@company.com inside
// otherwise-honest prose. With the deltas-only rule the routes for an email into
// this payload are the operator's own project name and the request path. That is
// a gap, not a promise.
func RedactPayload(p Payload) Payload {
	p.Endpoint = capture.RedactValue(p.Endpoint)

	deltas := make([]types.DiffDelta, len(p.Deltas))
	copy(deltas, p.Deltas)
	for i := range deltas {
		deltas[i].Message = capture.RedactValue(deltas[i].Message)
		deltas[i].JSONPath = capture.RedactValue(deltas[i].JSONPath)
		deltas[i].Expected = capture.RedactValue(deltas[i].Expected)
		deltas[i].Actual = capture.RedactValue(deltas[i].Actual)
	}
	p.Deltas = deltas

	return p
}
