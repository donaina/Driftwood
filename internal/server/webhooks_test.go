package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/webhook"
	"github.com/donaina/driftwood/pkg/types"
)

const (
	webhooksRoute   = "/_driftwood/api/webhooks"
	deleteRoute     = "/_driftwood/api/webhooks/delete"
	deliveriesRoute = "/_driftwood/api/webhooks/deliveries"
	// A URL that parses, is https, and names a host the SSRF rules allow. What
	// the route is asked to store is not what it dials, so nothing here has to
	// be listening.
	slackURL = "https://hooks.slack.com/services/T000/B000/XXXXXXXXXXXXXXXX"
)

// webhookViews is the response shape every route in this file returns.
type webhookViews []struct {
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	HasSecret bool      `json:"has_secret"`
	UpdatedAt time.Time `json:"updated_at"`
}

// readAndDecode reads the body once, returning both the bytes and the decoded
// value. The bytes are what the secret-absence assertions need: decoding tells
// you what the server meant to send, and the leak this guards against is a field
// that was sent without being meant to.
func readAndDecode(t *testing.T, resp *http.Response, v interface{}) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	if v != nil {
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatalf("decoding %s: %v", raw, err)
		}
	}
	return string(raw)
}

// A saved config comes back with the URL and the fact that a secret is set —
// and the secret itself appears nowhere in the response.
func TestWebhookConfigRoundTripsWithoutItsSecret(t *testing.T) {
	h := newHarness(t)
	const secret = "whsec-planted-signing-key"

	resp := h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"`+slackURL+
		`","enabled":true,"secret":"`+secret+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var saved webhookViews
	body := readAndDecode(t, resp, &saved)

	if strings.Contains(body, secret) {
		t.Error("the save response carried the signing secret back to the caller")
	}
	if len(saved) != 1 {
		t.Fatalf("the save response listed %d configs, want 1", len(saved))
	}
	if saved[0].URL != slackURL || !saved[0].Enabled || !saved[0].HasSecret {
		t.Errorf("saved config came back as %+v", saved[0])
	}

	// And it is readable afterwards, which is what the dashboard does on mount.
	var listed webhookViews
	body = readAndDecode(t, h.do(t, http.MethodGet, webhooksRoute, nil), &listed)
	if strings.Contains(body, secret) {
		t.Error("the GET carried the signing secret")
	}
	if len(listed) != 1 || listed[0].Kind != types.WebhookGeneric || listed[0].URL != slackURL {
		t.Errorf("GET returned %+v, want the generic config that was saved", listed)
	}
}

// A patch that names one field must not disturb the others. The secret is the
// one that matters most: a dashboard that sends only the URL on every Save would
// otherwise erase the signing key each time somebody pressed the button.
func TestWebhookPatchLeavesUnnamedFieldsAlone(t *testing.T) {
	h := newHarness(t)

	h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"`+slackURL+`","enabled":true,"secret":"keep-me"}`)

	// Enabled alone. No url, no secret: both must survive.
	resp := h.postJSON(t, webhooksRoute, `{"kind":"generic","enabled":false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var listed webhookViews
	readAndDecode(t, resp, &listed)
	if len(listed) != 1 {
		t.Fatalf("got %d configs, want 1", len(listed))
	}
	if listed[0].URL != slackURL {
		t.Errorf("URL = %q after a patch that did not name it; want it left alone", listed[0].URL)
	}
	if !listed[0].HasSecret {
		t.Error("the signing secret was cleared by a patch that did not name it")
	}
	if listed[0].Enabled {
		t.Error("enabled = true after a patch that set it false")
	}

	// An explicit empty string is the way to clear it, and it does.
	h.postJSON(t, webhooksRoute, `{"kind":"generic","secret":""}`)
	var cleared webhookViews
	readAndDecode(t, h.do(t, http.MethodGet, webhooksRoute, nil), &cleared)
	if cleared[0].HasSecret {
		t.Error(`secret sent as "" was not cleared; absent and empty are not distinguishable`)
	}
}

// A URL pointing at link-local metadata is refused from off-box, and refused as
// the client's problem rather than the server's.
//
// It carries the forwarded header for the same reason the private case below
// does, and this is worth being exact about: the rule is not "metadata is always
// refused". An operator at their own dashboard is allowed to name a private host
// — that is the local-development case this product is built around, and the same
// permission already lets them point the proxy at 169.254.169.254. What must not
// happen is a request that arrived over the wire aiming the deliverer there, so
// the assertion carries the evidence that something stood in front of the call.
func TestWebhookRefusesAMetadataURLFromAProxiedRequest(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSONWithHeaders(t, webhooksRoute,
		`{"kind":"generic","url":"http://169.254.169.254/latest/meta-data/","enabled":true}`,
		map[string]string{"X-Forwarded-For": "203.0.113.5"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a metadata URL from a proxied request, want 400", resp.StatusCode)
	}
	if got := h.store.GetWebhooks(h.store.ActiveProject()); len(got) != 0 {
		t.Errorf("a refused URL was stored anyway: %+v", got)
	}
}

// The rule is about who named the URL, not whether it is private.
//
// The same private URL is accepted from loopback and refused when a forwarded
// header says something stood in front of the request — which is the whole
// content of isLoopbackRequest, and the only trust boundary this product has.
func TestWebhookPrivateURLIsJudgedByWhereTheRequestCameFrom(t *testing.T) {
	h := newHarness(t)
	const private = "http://127.0.0.1:9099/hook"

	fromOperator := h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"`+private+`","enabled":true}`)
	if fromOperator.StatusCode != http.StatusOK {
		t.Errorf("status = %d saving a private URL from loopback, want 200 — this is a local "+
			"dev tool and its own default target is localhost", fromOperator.StatusCode)
	}

	// The same body, this time carrying the evidence that a reverse proxy stood
	// in front. A proxied request is not a local one, so the trust is withdrawn.
	proxied := h.postJSONWithHeaders(t, webhooksRoute,
		`{"kind":"discord","url":"`+private+`","enabled":true}`,
		map[string]string{"X-Forwarded-For": "203.0.113.5"})
	if proxied.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(proxied.Body)
		proxied.Body.Close()
		t.Errorf("status = %d saving a private URL from a proxied request, want 400; body: %s",
			proxied.StatusCode, body)
	}
}

// Reads are open: the dashboard fetches its config over the same connection it
// fetches everything else on, and there is nothing in a config a reader can act
// on. Only the URL check gates a write.
func TestWebhookReadIsNotGatedByOrigin(t *testing.T) {
	h := newHarness(t)
	h.postJSON(t, webhooksRoute, `{"kind":"slack","url":"`+slackURL+`","enabled":true}`)

	resp := h.do(t, http.MethodGet, webhooksRoute, map[string]string{"X-Forwarded-For": "203.0.113.5"})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d reading the config through a proxy, want 200", resp.StatusCode)
	}
}

func TestWebhookRoutesRefuseBadRequests(t *testing.T) {
	h := newHarness(t)
	h.postJSON(t, webhooksRoute, `{"kind":"slack","url":"`+slackURL+`","enabled":true}`)

	for _, tc := range []struct {
		name string
		body string
	}{
		// A config for a kind nothing renders is a setting with no effect.
		{"unknown kind", `{"kind":"carrier-pigeon","url":"` + slackURL + `","enabled":true}`},
		// Enabling a channel that points nowhere would show a card that is on
		// and does nothing.
		{"enabled with no URL", `{"kind":"teams","enabled":true}`},
		{"not JSON", `{`},
		{"a scheme that is not http", `{"kind":"teams","url":"file:///etc/passwd"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.postJSON(t, webhooksRoute, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}

	// And none of them wrote anything: the list is still the one config.
	if got := h.store.GetWebhooks(h.store.ActiveProject()); len(got) != 1 {
		t.Errorf("a refused request changed the store: %+v", got)
	}
}

func TestWebhookRoutesRefuseAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, webhooksRoute+"?project=nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET ?project=nope = %d, want 404", resp.StatusCode)
	}

	resp = h.postJSON(t, webhooksRoute+"?project=nope", `{"kind":"slack","url":"`+slackURL+`","enabled":true}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST ?project=nope = %d, want 404 — a config written under a project that does "+
			"not exist is one nothing will ever read", resp.StatusCode)
	}

	resp = h.postJSON(t, deleteRoute+"?project=nope", `{"kind":"slack"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST delete ?project=nope = %d, want 404", resp.StatusCode)
	}
}

func TestDeleteWebhookReturnsTheRemainingList(t *testing.T) {
	h := newHarness(t)
	h.postJSON(t, webhooksRoute, `{"kind":"slack","url":"`+slackURL+`","enabled":true}`)
	h.postJSON(t, webhooksRoute, `{"kind":"discord","url":"https://discord.com/api/webhooks/1/tok","enabled":true}`)

	resp := h.postJSON(t, deleteRoute, `{"kind":"slack"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var remaining webhookViews
	readAndDecode(t, resp, &remaining)
	if len(remaining) != 1 || remaining[0].Kind != types.WebhookDiscord {
		t.Errorf("after deleting slack the list is %+v, want only discord", remaining)
	}

	// Deleting a kind that was never configured is not an error: the caller
	// asked for it to be gone, and it is.
	if resp := h.postJSON(t, deleteRoute, `{"kind":"teams"}`); resp.StatusCode != http.StatusOK {
		t.Errorf("deleting an unconfigured kind = %d, want 200", resp.StatusCode)
	}

	if resp := h.postJSON(t, deleteRoute, `{"kind":"carrier-pigeon"}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("deleting an unknown kind = %d, want 400", resp.StatusCode)
	}
}

func TestWebhookRoutesRejectOtherMethods(t *testing.T) {
	h := newHarness(t)

	if resp := h.do(t, http.MethodPut, webhooksRoute, nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT %s = %d, want 405", webhooksRoute, resp.StatusCode)
	}
	if resp := h.do(t, http.MethodGet, deleteRoute, nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET %s = %d, want 405 — the delete route is a POST, and a GET that deleted "+
			"something would be a link a browser could follow", deleteRoute, resp.StatusCode)
	}
}

// The change reaches the dashboard without a reload, the way every other change
// to project state does.
func TestWebhookChangeIsAnnouncedOnTheHub(t *testing.T) {
	h := newHarness(t)
	active := h.store.ActiveProject()

	resp, err := http.Get(h.router.URL + "/_driftwood/events?project=" + active)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()

	frames := make(chan map[string]interface{}, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) == nil && frame["type"] != nil {
				frames <- frame
			}
		}
	}()

	// The subscriber is registered by the time the ping has been written, so the
	// wait is about the ping reaching us rather than about the hub.
	time.Sleep(100 * time.Millisecond)

	h.postJSON(t, webhooksRoute, `{"kind":"slack","url":"`+slackURL+`","enabled":true}`)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame := <-frames:
			if frame["type"] != "webhook_updated" {
				continue
			}
			list, ok := frame["data"].([]interface{})
			if !ok || len(list) != 1 {
				t.Fatalf("webhook_updated carried %v, want the list with one config", frame["data"])
			}
			entry, _ := list[0].(map[string]interface{})
			if entry["kind"] != types.WebhookSlack || entry["url"] != slackURL {
				t.Errorf("the announced config was %v", entry)
			}
			if _, leaked := entry["secret"]; leaked {
				t.Error("the announced config carried a secret field")
			}
			return
		case <-deadline:
			t.Fatal("saving a webhook published nothing on the stream")
		}
	}
}

// TestWebhookRecordsThePrivateURLVerdict covers the field the deliverer depends
// on, and the two things that can make it wrong.
//
// The verdict is not re-derived at delivery time — the deliverer has no request
// to look at — so it has to be recorded when the URL is saved, and cleared when
// the URL is. A stale permission on a config whose URL changed would let a later
// URL inherit a decision made about a different address.
func TestWebhookRecordsThePrivateURLVerdict(t *testing.T) {
	h := newHarness(t)

	// A public URL saved from loopback. The rule permits a private address from
	// here, but this one is not private, and the flag follows the address rather
	// than the request.
	h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"`+slackURL+`","enabled":true}`)
	if cfg, ok := h.store.GetWebhook(h.store.ActiveProject(), "generic"); !ok || cfg.AllowPrivate {
		t.Errorf("a public URL was saved with allow_private set: %+v", cfg)
	}

	// A loopback URL from loopback. This is the local receiver case, and the
	// flag is what keeps the deliverer's dial from refusing it forever.
	h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"http://127.0.0.1:9099/hook","enabled":true}`)
	cfg, ok := h.store.GetWebhook(h.store.ActiveProject(), "generic")
	if !ok {
		t.Fatal("the loopback URL was not saved")
	}
	if !cfg.AllowPrivate {
		t.Error("a loopback URL saved from loopback was not permitted, so every delivery to it will be refused at the dial")
	}

	// And a URL that arrives over the wire never carries the permission, even
	// though the address is the same one that was just permitted.
	resp := h.postJSONWithHeaders(t, webhooksRoute,
		`{"kind":"generic","url":"http://127.0.0.1:9099/hook","enabled":true}`,
		map[string]string{"X-Forwarded-For": "203.0.113.7"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	readAndDecode(t, resp, nil)

	cfg, _ = h.store.GetWebhook(h.store.ActiveProject(), "generic")
	if cfg.AllowPrivate != true {
		// Refused, so the stored config is untouched — the refusal must not have
		// written anything.
		t.Error("a refused URL changed the stored config")
	}

	// Clearing the URL clears the permission with it.
	h.postJSON(t, webhooksRoute, `{"kind":"generic","url":"","enabled":false}`)
	if cfg, _ := h.store.GetWebhook(h.store.ActiveProject(), "generic"); cfg.AllowPrivate {
		t.Error("clearing the URL left the private-address permission behind")
	}
}

// TestDeliveriesRouteReportsRecords: the read side of the deliverer, wired to
// the server. The fake log is the server's own seam — the route serves records
// and never causes a delivery, so constructing a real deliverer here would start
// workers to exercise none of them.
func TestDeliveriesRouteReportsRecords(t *testing.T) {
	h := newHarness(t)
	active := h.store.ActiveProject()

	h.deliveries.records = []webhook.Record{
		{
			ID: "wd_1", ProjectID: active, Kind: types.WebhookSlack,
			Endpoint: "GET /api/users", Status: webhook.StatusDelivered,
			Attempts: 1, StatusCode: 200,
		},
		{
			ID: "wd_2", ProjectID: active, Kind: types.WebhookDiscord,
			Endpoint: "GET /api/users", Status: webhook.StatusFailed,
			Attempts: 3, StatusCode: 500, Error: "the receiver refused the payload",
		},
		// Another project's record, which must not appear.
		{ID: "wd_3", ProjectID: "someone-else", Status: webhook.StatusDelivered},
	}

	var records []webhook.Record
	body := readAndDecode(t, h.do(t, http.MethodGet, deliveriesRoute, nil), &records)

	if len(records) != 2 {
		t.Fatalf("the route returned %d records, want 2 for this project: %s", len(records), body)
	}
	if records[0].Status != webhook.StatusDelivered || records[1].Status != webhook.StatusFailed {
		t.Errorf("the records came back as %+v", records)
	}
	// The failed one has to carry what an operator acts on.
	if records[1].Attempts != 3 || records[1].StatusCode != 500 || records[1].Error == "" {
		t.Errorf("the failed record lost its detail: %+v", records[1])
	}
	if h.deliveries.limit != deliveriesLimit {
		t.Errorf("the route asked for %d records, want the bounded %d", h.deliveries.limit, deliveriesLimit)
	}
}

// An empty log is an empty list, not null and not a 500 — the dashboard renders
// an EmptyState from it, and a null would make that state unreachable.
func TestDeliveriesRouteIsEmptyBeforeAnythingIsSent(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, deliveriesRoute, nil)
	body := readAndDecode(t, resp, nil)

	if strings.TrimSpace(body) != "[]" {
		t.Errorf("an empty delivery log answered %q, want []", strings.TrimSpace(body))
	}
}

func TestDeliveriesRouteRefusesOtherMethods(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, deliveriesRoute, `{}`)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST on the deliveries route = %d, want 405", resp.StatusCode)
	}
	readAndDecode(t, resp, nil)
}

func TestDeliveriesRouteRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, deliveriesRoute+"?project=nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	readAndDecode(t, resp, nil)
}
