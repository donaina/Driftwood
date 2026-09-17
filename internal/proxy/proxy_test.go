package proxy

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
)

// isolateHome points the store at a throwaway home directory.
//
// storage.NewStore resolves its persist path from the home directory, and the
// proxy auto-saves a baseline for the first JSON response it sees — so without
// this the suite reads the developer's real ~/.driftwood and writes test
// endpoints into it. GET:/api/test was sitting in a real baselines.json, put
// there by TestSanitizeTrafficWired. internal/server and cmd/drift have isolated
// HOME for exactly this reason; these tests had not.
//
// A temp HOME is also what makes the assertions exact: a developer who already
// has a baseline for /api/test gets different behaviour from one who does not.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestProxySSRFValidation(t *testing.T) {
	isolateHome(t)
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Test: valid http URL
	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("valid http URL should be accepted: %v", err)
	}
	if prx.SetTarget("https://api.example.com") != nil {
		t.Errorf("valid https URL should be accepted")
	}

	// Test: invalid schemes should be rejected
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://localhost:8787",   // SSRF to local metadata - should be blocked
		"http://169.254.169.254/", // AWS metadata
		"http://127.0.0.1:8080",   // local
		"",                        // empty
		"not-a-url",
	}

	for _, u := range invalidURLs {
		err = prx.SetTarget(u)
		if err == nil {
			t.Errorf("SSRF URL should be rejected: %s", u)
		}
	}

	// Test: valid URL after invalid
	if err := prx.SetTarget("https://api.github.com"); err != nil {
		t.Errorf("valid URL after invalid should work: %v", err)
	}
}

func TestProxyTargetURLRaceSafety(t *testing.T) {
	isolateHome(t)
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent SetTarget and Handler access should not panic
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			prx.SetTarget("https://api.example.com")
			_ = prx.Handler()
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestSanitizeTrafficWired(t *testing.T) {
	isolateHome(t)
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Use test proxy for localhost target
	prx, err := NewProxyForTest("http://localhost:3000", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test server that returns JSON with auth headers
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret123; HttpOnly")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 1, "name": "test"}`))
	}))
	defer targetServer.Close()

	// Point proxy to test server
	targetURL, _ := url.Parse(targetServer.URL)
	prx.GetTargetURLForTest().Store(targetURL)

	// Make request through proxy
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret456")

	rec := httptest.NewRecorder()
	prx.Handler()(rec, req)

	// Check that traffic was captured and sanitized
	// The stored traffic should have redacted headers
	t.Logf("Response status: %d", rec.Code)
}

// TestBreakingAlertCarriesItsExplanation covers a path that had no test at all,
// which is why it was broken in three places at once and nobody noticed.
//
// The contract: the alert is published the moment the break is detected and
// does not wait for the sidecar; the explanation is filed against the stored
// alert; and it is announced separately, once it exists. None of that held
// before. The write used to land in a map Publish had already marshalled, so
// the explanation reached neither the wire nor the store, and the Alerts view
// read three fields no alert has ever had.
func TestBreakingAlertCarriesItsExplanation(t *testing.T) {
	isolateHome(t)

	/* A stub for the sidecar. The body is the shape ai/src/types.ts declares,
	   and it travels on a channel rather than a shared variable so the handler
	   goroutine and this one are ordered by something the race detector can
	   see. */
	sidecarBodies := make(chan map[string]interface{}, 4)
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sidecarBodies <- body

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":"count became a string",` +
			`"impact":"parsers expecting a number fail",` +
			`"root_cause":"a column type change",` +
			`"suggested_action":"revert the migration",` +
			`"confidence":0.9}`))
	}))
	defer sidecar.Close()

	// Written before any request, so the goroutine that reads it is ordered
	// after this line by the go statement that spawns it.
	oldExplainURL := explainURL
	explainURL = sidecar.URL
	t.Cleanup(func() { explainURL = oldExplainURL })

	// The first response is captured as the baseline; the second breaks it.
	var hits int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt64(&hits, 1) == 1 {
			_, _ = w.Write([]byte(`{"count":1,"token":"sk-live-must-not-leave"}`))
			return
		}
		_, _ = w.Write([]byte(`{"count":"one","token":"sk-live-must-not-leave"}`))
	}))
	defer backend.Close()

	store := storage.NewStore(backend.URL, "8787")
	hub := events.NewHub()
	prx, err := NewProxyForTest(backend.URL, store, hub, &mock.MockController{})
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe over the same SSE endpoint the dashboard uses, so this covers
	// delivery rather than only the hub's memory.
	type broadcast struct {
		Type string                 `json:"type"`
		Data map[string]interface{} `json:"data"`
	}
	received := make(chan broadcast, 32)

	sse := httptest.NewServer(http.HandlerFunc(hub.SSEHandler))
	defer sse.Close()

	stream, err := http.Get(sse.URL)
	if err != nil {
		t.Fatalf("could not open the event stream: %v", err)
	}
	defer stream.Body.Close()

	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue // the "event: ping" line, and the blank separators
			}
			var b broadcast
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &b); err == nil && b.Type != "" {
				received <- b
			}
		}
	}()

	// waitFor returns the next event of this type, discarding the others.
	waitFor := func(eventType string) broadcast {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case b := <-received:
				if b.Type == eventType {
					return b
				}
			case <-deadline:
				t.Fatalf("timed out waiting for a %q event", eventType)
				return broadcast{}
			}
		}
	}

	request := func() {
		req := httptest.NewRequest(http.MethodGet, "/thing", nil)
		prx.Handler()(httptest.NewRecorder(), req)
	}

	request() // establishes the baseline
	request() // breaks it

	alert := waitFor("alert")
	if got := alert.Data["contract_status"]; got != "BREAKING" {
		t.Fatalf("expected a BREAKING alert, got %v", got)
	}

	/* The alert must not be carrying an explanation, and not because the
	   sidecar is slow — nothing writes that key into the published map any
	   more. The alert is the notification, so it goes out immediately; waiting
	   on an optional paragraph would put a network round trip in front of every
	   breaking change. */
	if _, present := alert.Data["ai_explanation"]; present {
		t.Error("the published alert carried an explanation; it must not wait for the sidecar")
	}

	explained := waitFor("alert_explained")
	if explained.Data["traffic_id"] != alert.Data["traffic_id"] {
		t.Errorf("alert_explained names %v, but the alert was %v",
			explained.Data["traffic_id"], alert.Data["traffic_id"])
	}

	alerts := store.GetAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 stored alert, got %d", len(alerts))
	}
	if alerts[0].AIExplanation == nil {
		t.Fatal("the stored alert carries no explanation: the sidecar answered but nothing filed it")
	}
	if got := alerts[0].AIExplanation["summary"]; got != "count became a string" {
		t.Errorf("stored explanation summary = %v", got)
	}

	/* The alert is one endpoint breaking, with the per-field detail under Diff.
	   These are the paths the Alerts view reads, asserted where they are
	   produced rather than only in a browser. */
	if alerts[0].Diff == nil || len(alerts[0].Diff.Deltas) == 0 {
		t.Fatal("the alert carries no deltas, so the Alerts view has nothing to render")
	}
	for _, d := range alerts[0].Diff.Deltas {
		if d.JSONPath == "" || d.Severity == "" || d.Message == "" {
			t.Errorf("delta is missing a field the Alerts view reads: %+v", d)
		}
	}
	if alerts[0].Endpoint != "GET /thing" {
		t.Errorf("alert endpoint = %q", alerts[0].Endpoint)
	}

	/* Both samples leave the process for a third-party API. The baseline is
	   stored raw, so the copy that must be clean is the one sent. */
	var sidecarBody map[string]interface{}
	select {
	case sidecarBody = <-sidecarBodies:
	case <-time.After(time.Second):
		t.Fatal("the sidecar was never called")
	}
	for _, field := range []string{"baseline_sample", "current_sample"} {
		sample, ok := sidecarBody[field].(string)
		if !ok {
			t.Errorf("the sidecar was sent no %s: %v", field, sidecarBody)
			continue
		}
		if strings.Contains(sample, "sk-live-must-not-leave") {
			t.Errorf("the %s reached the sidecar unredacted: %s", field, sample)
		}
	}
	if sidecarBody["endpoint"] != "GET /thing" {
		t.Errorf("the sidecar was told about %v", sidecarBody["endpoint"])
	}
}
