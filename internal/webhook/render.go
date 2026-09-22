package webhook

/* One payload, four bodies.

   Everything the four channels share lives above this file: the alert becomes a
   Payload once, and the only thing a kind decides is how to wrap it. That is the
   entire justification for a "kind" existing rather than four delivery
   mechanisms — Slack, Teams, Discord and a generic POST are the same operation
   with a different envelope.

   Every rendering is bounded. A single KindAddedField delta against a large
   object can produce hundreds of entries, so the over-limit case is the common
   one rather than the edge — and an over-limit body is a 400 from the vendor
   that looks exactly like a delivery bug. Slack caps text at 4000 characters and
   Discord at 2000; both are enforced by the vendor, not by us. */

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/donaina/driftwood/pkg/types"
)

// maxDeltas bounds how many deltas any rendering names before summarising the
// rest. Twenty is roughly a screenful in every one of the four clients, and it
// keeps even a pathological diff inside Discord's 2000-character content limit
// for the common case.
const maxDeltas = 20

// Render turns a payload into the body one kind expects.
//
// The second return is the content type, which is not always
// application/json in spirit: Slack reads a body sent as text/plain as a form
// submission and answers invalid_payload, so the header is part of the
// rendering rather than a detail of the transport.
func Render(kind string, p Payload) (body []byte, contentType string, err error) {
	var v interface{}

	switch kind {
	case types.WebhookSlack:
		v = slackBody(p)
	case types.WebhookTeams:
		v = teamsBody(p)
	case types.WebhookDiscord:
		v = discordBody(p)
	case types.WebhookGeneric:
		v = p
	default:
		return nil, "", fmt.Errorf("no rendering for webhook kind %q", kind)
	}

	body, err = json.Marshal(v)
	if err != nil {
		return nil, "", fmt.Errorf("rendering the %s body: %w", kind, err)
	}
	return body, "application/json", nil
}

// headline is the one-line description every human-facing rendering leads with.
func headline(p Payload) string {
	verb := "changed"
	switch p.Event {
	case EventTest:
		verb = "test alert"
	case EventContractDrift:
		if strings.EqualFold(p.Status, string(types.SeverityBreaking)) {
			verb = "broke"
		}
	}
	return fmt.Sprintf("%s %s", p.Endpoint, verb)
}

// summarize builds the multi-line human body, shared by the three chat kinds.
//
// The counts come from Summary rather than from re-walking Deltas, so the number
// in the prose and the list underneath it cannot disagree.
func summarize(p Payload) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", headline(p))
	fmt.Fprintf(&b, "project: %s\n", p.Project.Name)
	fmt.Fprintf(&b, "status: %s\n", p.Status)

	counts := make([]string, 0, 3)
	for _, c := range []struct {
		n     int
		label string
	}{
		{p.Summary.Breaking, "breaking"},
		{p.Summary.Warning, "warning"},
		{p.Summary.Info, "info"},
	} {
		if c.n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", c.n, c.label))
		}
	}
	if len(counts) > 0 {
		fmt.Fprintf(&b, "deltas: %s\n", strings.Join(counts, ", "))
	}

	shown := p.Deltas
	if len(shown) > maxDeltas {
		shown = shown[:maxDeltas]
	}
	for _, d := range shown {
		fmt.Fprintf(&b, "\n- %s %s\n  %s", d.Severity, d.JSONPath, d.Message)
	}
	if rest := p.Summary.Total - len(shown); rest > 0 {
		fmt.Fprintf(&b, "\n\n…and %d more (see the Contract Alerts view)", rest)
	}

	return b.String()
}

// slackBody is mrkdwn, which is not markdown: *bold* is one asterisk and a
// bullet is a literal •. The two render differently enough that reusing the
// generic text would produce visible artifacts.
func slackBody(p Payload) map[string]interface{} {
	var b strings.Builder
	fmt.Fprintf(&b, "*%s*\n", headline(p))
	fmt.Fprintf(&b, "Project %s · %s\n", p.Project.Name, p.Status)

	shown := p.Deltas
	if len(shown) > maxDeltas {
		shown = shown[:maxDeltas]
	}
	for _, d := range shown {
		fmt.Fprintf(&b, "\n• `%s` %s\n  %s", d.JSONPath, d.Severity, d.Message)
	}
	if rest := p.Summary.Total - len(shown); rest > 0 {
		fmt.Fprintf(&b, "\n\n…and %d more (see the Contract Alerts view)", rest)
	}

	// Slack rejects a text longer than 4000 characters. Truncating here rather
	// than letting the vendor answer 400 keeps a large diff from being reported
	// as a delivery failure, which is what it would otherwise look like.
	return map[string]interface{}{"text": truncate(b.String(), 3900)}
}

// teamsBody is an Adaptive Card in an attachments envelope, which is the Power
// Automate workflow webhook shape — Microsoft retired the Office 365 connector
// that took the older MessageCard.
//
// The envelope is the one part of this file that has not been exercised against
// a real tenant. A wrong envelope is a 400 at delivery time and nothing else,
// which is why the test route exists: an operator finds out in seconds rather
// than discovering it when the first contract breaks. Treat it as unverified
// until a real workflow has accepted one.
func teamsBody(p Payload) map[string]interface{} {
	body := make([]map[string]interface{}, 0, maxDeltas+2)
	body = append(body,
		map[string]interface{}{
			"type": "TextBlock", "size": "Medium", "weight": "Bolder",
			"text": headline(p), "wrap": true,
		},
		map[string]interface{}{
			"type": "TextBlock", "isSubtle": true, "spacing": "None",
			"text": fmt.Sprintf("Project %s · %s · %d deltas", p.Project.Name, p.Status, p.Summary.Total),
			"wrap": true,
		},
	)

	shown := p.Deltas
	if len(shown) > maxDeltas {
		shown = shown[:maxDeltas]
	}
	for _, d := range shown {
		body = append(body, map[string]interface{}{
			"type": "TextBlock", "wrap": true,
			"text": fmt.Sprintf("%s  %s\n%s", d.Severity, d.JSONPath, d.Message),
		})
	}
	if rest := p.Summary.Total - len(shown); rest > 0 {
		body = append(body, map[string]interface{}{
			"type": "TextBlock", "isSubtle": true, "wrap": true,
			"text": fmt.Sprintf("…and %d more (see the Contract Alerts view)", rest),
		})
	}

	return map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]interface{}{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"type":    "AdaptiveCard",
				"version": "1.4",
				"body":    body,
			},
		}},
	}
}

// discordBody posts as a named webhook user, because an unattributed message in
// a channel reads as though a person typed it.
func discordBody(p Payload) map[string]interface{} {
	return map[string]interface{}{
		"content":  truncate(summarize(p), 1900),
		"username": "Driftwood",
	}
}

// truncate cuts s to at most n bytes on a rune boundary, marking the cut so a
// reader can tell a shortened message from a complete one.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const marker = "\n…(truncated)"
	cut := n - len(marker)
	if cut < 0 {
		return s[:n]
	}
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + marker
}

// utf8Start reports whether b begins a UTF-8 sequence rather than continuing
// one, so truncate cannot split a multi-byte character in half.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
