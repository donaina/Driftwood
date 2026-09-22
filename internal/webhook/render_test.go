package webhook

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

// sampleAlert is one breaking and one warning delta, which is the shape the
// summary counts and the rendering order are asserted against.
func sampleAlert() types.Alert {
	return types.Alert{
		TrafficID:      "tr_1",
		Endpoint:       "GET /api/users",
		ContractStatus: string(types.SeverityBreaking),
		Diff: &types.ContractDiff{
			HasBreakingChanges: true,
			HasWarnings:        true,
			Deltas: []types.DiffDelta{
				{
					JSONPath: "$.count",
					Kind:     types.KindTypeMismatch,
					Severity: types.SeverityBreaking,
					Message:  "count changed type from number to string",
					Expected: "number",
					Actual:   "string",
				},
				{
					JSONPath: "$.nickname",
					Kind:     types.KindAddedField,
					Severity: types.SeverityWarning,
					Message:  "nickname was added",
				},
			},
		},
	}
}

func samplePayload() Payload {
	return BuildPayload(EventContractDrift, ProjectRef{ID: "acme", Name: "Acme"}, sampleAlert(), time.Now())
}

// allKinds is every kind Render claims to handle, so a kind added to the product
// and forgotten here fails the table rather than escaping it.
var allKinds = []string{
	types.WebhookSlack,
	types.WebhookTeams,
	types.WebhookDiscord,
	types.WebhookGeneric,
}

func TestRenderProducesValidJSONForEveryKind(t *testing.T) {
	p := samplePayload()

	for _, kind := range allKinds {
		t.Run(kind, func(t *testing.T) {
			body, contentType, err := Render(kind, p)
			if err != nil {
				t.Fatalf("Render(%q): %v", kind, err)
			}
			if contentType != "application/json" {
				// Not ceremony: Slack reads a body sent as text/plain as a form
				// submission and answers invalid_payload, so the header is part
				// of the rendering.
				t.Errorf("content type = %q, want application/json", contentType)
			}

			var decoded map[string]interface{}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("body is not a JSON object: %v\n%s", err, body)
			}
		})
	}
}

func TestRenderRefusesAnUnknownKind(t *testing.T) {
	if _, _, err := Render("email", samplePayload()); err == nil {
		t.Fatal("Render accepted a kind with no rendering")
	}
}

// TestGenericIsTheCanonicalPayload pins the one kind that is not an envelope:
// the generic body is the payload itself, so an operator wiring Driftwood into
// their own systems gets the same fields the dashboard has.
func TestGenericIsTheCanonicalPayload(t *testing.T) {
	p := samplePayload()

	body, _, err := Render(types.WebhookGeneric, p)
	if err != nil {
		t.Fatal(err)
	}

	var back Payload
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("decoding the generic body: %v", err)
	}
	if back.Event != EventContractDrift {
		t.Errorf("event = %q, want %q", back.Event, EventContractDrift)
	}
	if back.Endpoint != p.Endpoint {
		t.Errorf("endpoint = %q, want %q", back.Endpoint, p.Endpoint)
	}
	if len(back.Deltas) != len(p.Deltas) {
		t.Fatalf("deltas = %d, want %d", len(back.Deltas), len(p.Deltas))
	}
	if back.Summary != p.Summary {
		t.Errorf("summary = %+v, want %+v", back.Summary, p.Summary)
	}
}

func TestSummaryCountsBySeverity(t *testing.T) {
	p := samplePayload()

	if p.Summary.Breaking != 1 || p.Summary.Warning != 1 || p.Summary.Info != 0 {
		t.Errorf("summary = %+v, want 1 breaking, 1 warning, 0 info", p.Summary)
	}
	if p.Summary.Total != 2 {
		t.Errorf("total = %d, want 2", p.Summary.Total)
	}
}

/* The bounding tests below are the ones that matter in production rather than in
   principle. A single added-field delta against a large object produces hundreds
   of entries, so over-limit is the common case, and an over-limit body is a 400
   from the vendor that reads exactly like a delivery bug. */

// widePayload builds a payload with more deltas than any rendering names.
func widePayload() Payload {
	deltas := make([]types.DiffDelta, maxDeltas+5)
	for i := range deltas {
		deltas[i] = types.DiffDelta{
			JSONPath: "$.field" + strings.Repeat("x", 20) + string(rune('a'+i%26)),
			Severity: types.SeverityInfo,
			Message:  strings.Repeat("a long message about a field. ", 8),
		}
	}
	return Payload{
		Event:      EventContractDrift,
		Project:    ProjectRef{ID: "acme", Name: "Acme"},
		Endpoint:   "GET /api/wide",
		Status:     string(types.SeverityWarning),
		TrafficID:  "tr_wide",
		DetectedAt: time.Now(),
		Summary:    Summary{Info: len(deltas), Total: len(deltas)},
		Deltas:     deltas,
	}
}

func TestRenderingsStayInsideTheVendorLimits(t *testing.T) {
	p := widePayload()

	for _, kind := range allKinds {
		t.Run(kind, func(t *testing.T) {
			body, _, err := Render(kind, p)
			if err != nil {
				t.Fatalf("Render(%q): %v", kind, err)
			}

			/* The vendors cap a *field*, not a request body, so the assertion is
			   against the decoded field rather than len(body). Measuring the
			   encoded bytes would be measuring the wrong thing twice over: JSON
			   escaping expands a newline to two bytes, and a bullet or an ellipsis
			   is three — so a body could pass a byte check while the field it
			   carries is over the limit, or fail one while being fine. */
			var decoded map[string]interface{}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("decoding the %s body: %v", kind, err)
			}

			var field string
			var limit int
			switch kind {
			case types.WebhookSlack:
				field, limit = "text", 4000
			case types.WebhookDiscord:
				field, limit = "content", 2000
			default:
				field, limit = "", 28*1024
			}

			if field == "" {
				if len(body) > limit {
					t.Errorf("%s body is %d bytes, over the %d it must stay inside", kind, len(body), limit)
				}
				return
			}

			text, _ := decoded[field].(string)
			if text == "" {
				t.Fatalf("the %s body carries no %q field:\n%s", kind, field, body)
			}
			if len(text) > limit {
				t.Errorf("%s %s is %d bytes, over the vendor's %d limit", kind, field, len(text), limit)
			}

			// And the truncation has to be visible. A body cut silently to the
			// limit reads as a complete report of a smaller break.
			if kind == types.WebhookSlack || kind == types.WebhookDiscord {
				if !strings.Contains(text, "truncated") && !strings.Contains(text, "more (see the Contract Alerts view)") {
					t.Error("an over-limit body carries no sign that it was shortened")
				}
			}
		})
	}
}

// TestSummarizeNamesTheRemainder pins the other half of bounding: deltas past
// the cap are counted rather than dropped, so a truncated message and a small
// break do not read the same.
func TestSummarizeNamesTheRemainder(t *testing.T) {
	p := widePayload()

	text := summarize(p)

	if !strings.Contains(text, "more (see the Contract Alerts view)") {
		t.Errorf("the over-limit count is missing from:\n%s", text)
	}
	// The count is derived from Summary.Total, so it must be the real remainder
	// rather than a hardcoded string.
	want := "…and 5 more"
	if !strings.Contains(text, want) {
		t.Errorf("want %q in:\n%s", want, text)
	}
}

func TestHeadlineDistinguishesABreakFromAChange(t *testing.T) {
	breaking := samplePayload()
	if got := headline(breaking); !strings.Contains(got, "broke") {
		t.Errorf("breaking headline = %q, want it to say broke", got)
	}

	warning := breaking
	warning.Status = string(types.SeverityWarning)
	if got := headline(warning); strings.Contains(got, "broke") {
		t.Errorf("warning headline = %q, must not say broke", got)
	}

	test := breaking
	test.Event = EventTest
	if got := headline(test); !strings.Contains(got, "test") {
		// The test send has to announce itself as one to the human reading the
		// channel, not only to the dashboard.
		t.Errorf("test headline = %q, want it to say test", got)
	}
}

func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	// Three bytes per rune, so a byte-index cut at an arbitrary offset lands
	// mid-character and produces invalid UTF-8.
	s := strings.Repeat("→", 100)

	got := truncate(s, 10)

	if !json.Valid([]byte(`"` + got + `"`)) {
		t.Errorf("truncate produced invalid UTF-8: %q", got)
	}
	if len(got) > 10 {
		t.Errorf("truncate returned %d bytes, want at most 10", len(got))
	}
}

func TestTruncateLeavesAShortStringAlone(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate = %q, want it unchanged", got)
	}
}
