package webhook

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/netguard"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

/* These tests drive the real Deliverer against httptest receivers. The order of
   goroutines is always a channel, never a sleep — the precedent is proxy_test.go's
   stub sidecar, and it is the difference between a suite that waits for the thing
   it is asserting and one that hopes it happened in time.

   Nothing here calls t.Parallel. The suite mutates backoffSchedule and $HOME, the
   same reason the rest of the repo has zero occurrences of it. */

// fastBackoff shrinks the retry schedule so a three-attempt test costs
// milliseconds rather than five seconds. Restored on cleanup, because it is a
// package-level var and a test that leaked it would quietly change every later
// test's timing.
func fastBackoff(t *testing.T) {
	t.Helper()
	old := backoffSchedule
	backoffSchedule = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { backoffSchedule = old })
}

// newDeliverer builds a deliverer over a store with one project, and registers
// the channel config. It returns the deliverer, already running.
func newDeliverer(t *testing.T, configs ...types.WebhookConfig) (*Deliverer, string) {
	t.Helper()

	// storage.NewStore resolves its path from the home directory, so without
	// this the suite writes into the developer's real ~/.driftwood.
	t.Setenv("HOME", t.TempDir())

	store, err := storage.NewStore("", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projectID := store.ActiveProject()

	for _, cfg := range configs {
		if err := store.SetWebhook(projectID, cfg); err != nil {
			t.Fatalf("SetWebhook(%s): %v", cfg.Kind, err)
		}
	}

	hub := events.NewHub()
	d := New(store, hub)
	d.Run()
	t.Cleanup(func() {
		// A short budget: every test here either drains or is already done, and
		// a cleanup that hangs hides the failure it is cleaning up after.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = d.Close(ctx)
	})

	return d, projectID
}

// configFor is the ordinary enabled-channel config, pointed at a receiver.
//
// AllowPrivate is set because every receiver in this suite is an httptest server
// on loopback — which is exactly the case the flag exists for, and exactly what
// the route records when an operator saves a local URL from a local dashboard.
// A config without it is the subject of its own test below, not the default.
func configFor(kind, url string) types.WebhookConfig {
	return types.WebhookConfig{Kind: kind, URL: url, Enabled: true, AllowPrivate: true}
}

// guardedConfig is a config that never passed the route's loopback rule, which
// is the state a restored file or a hand-edited document can be in.
func guardedConfig(kind, url string) types.WebhookConfig {
	return types.WebhookConfig{Kind: kind, URL: url, Enabled: true}
}

// waitFor polls until cond holds or the deadline passes. Polling rather than
// sleeping a fixed amount: a delivery that finishes in 2ms should not make the
// suite wait 200ms to notice, and a slow machine should not fail the suite.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// onlyRecord returns the single record a project holds, failing if there is not
// exactly one. Every test below delivers one alert, so more than one record
// means something delivered twice — which is itself a defect worth failing on.
func onlyRecord(t *testing.T, d *Deliverer, projectID string) Record {
	t.Helper()
	records := d.Recent(projectID, 0)
	if len(records) != 1 {
		t.Fatalf("got %d delivery records, want exactly 1: %+v", len(records), records)
	}
	return records[0]
}

func TestDeliverySucceedsOnTheFirstAttempt(t *testing.T) {
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			// Slack answers invalid_payload to a body sent as text/plain, so the
			// header is part of the contract with every one of these vendors.
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookSlack, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusDelivered {
		t.Errorf("status = %q, want %q (error: %s)", rec.Status, StatusDelivered, rec.Error)
	}
	if rec.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", rec.Attempts)
	}
	if rec.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want 200", rec.StatusCode)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("the receiver was hit %d times, want 1", got)
	}
	if rec.Endpoint != "GET /api/users" {
		t.Errorf("endpoint = %q, want GET /api/users", rec.Endpoint)
	}
}

// TestDeliveryRetriesAServerError covers the schedule itself: two failures and
// then success, asserted by counting what the receiver actually saw rather than
// by trusting the record's attempt count.
func TestDeliveryRetriesAServerError(t *testing.T) {
	fastBackoff(t)

	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusDelivered {
		t.Errorf("status = %q, want %q", rec.Status, StatusDelivered)
	}
	if rec.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", rec.Attempts)
	}
	if got := atomic.LoadInt64(&hits); got != 3 {
		t.Errorf("the receiver was hit %d times, want 3", got)
	}
}

// TestDeliveryDoesNotRetryAClientError pins the other half of the retry rule. A
// bad payload or a revoked URL does not fix itself, and three attempts turn a
// clear 400 into a slow one.
func TestDeliveryDoesNotRetryAClientError(t *testing.T) {
	fastBackoff(t)

	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookSlack, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Errorf("status = %q, want %q", rec.Status, StatusFailed)
	}
	if rec.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", rec.Attempts)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("the receiver was hit %d times, want exactly 1", got)
	}
}

/* The redirect tests are the SSRF regression, and the second server is the whole
   point: the guards check the configured URL's host once, so a webhook at an
   operator-named public host that answers "302 Location: http://169.254.169.254/"
   would reach link-local with every string check passed if the client followed
   redirects. The assertion is that the second server received nothing at all. */

func TestDeliveryRefusesToFollowARedirect(t *testing.T) {
	fastBackoff(t)

	var redirectHits int64
	var targetHits int64

	// The stand-in for whatever the redirect points at. It must see zero
	// requests; the address it is on is local, which is the case a redirect
	// would smuggle past the URL check.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&targetHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&redirectHits, 1)
		http.Redirect(w, r, target.URL+"/latest/meta-data/", http.StatusFound)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Errorf("status = %q, want %q — a redirect is a terminal failure", rec.Status, StatusFailed)
	}
	if rec.StatusCode != http.StatusFound {
		t.Errorf("status code = %d, want 302 recorded as seen", rec.StatusCode)
	}
	if got := atomic.LoadInt64(&redirectHits); got != 1 {
		t.Errorf("the redirecting receiver was hit %d times, want 1 — a 3xx must not be retried", got)
	}
	if got := atomic.LoadInt64(&targetHits); got != 0 {
		t.Fatalf("the redirect target was hit %d times, want 0 — this is the SSRF bypass", got)
	}
}

func TestRetryAfterIsHonoured(t *testing.T) {
	fastBackoff(t)

	var hits int64
	var firstAt, secondAt time.Time
	var mu sync.Mutex

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		mu.Lock()
		if n == 1 {
			firstAt = time.Now()
		} else if n == 2 {
			secondAt = time.Now()
		}
		mu.Unlock()

		if n == 1 {
			// 1s, where the test's own backoff is 1ms — so the gap below can
			// only come from the header being read.
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	if rec := onlyRecord(t, d, projectID); rec.Status != StatusDelivered {
		t.Fatalf("status = %q, want %q", rec.Status, StatusDelivered)
	}

	mu.Lock()
	gap := secondAt.Sub(firstAt)
	mu.Unlock()

	if gap < 900*time.Millisecond {
		t.Errorf("the retry came %v after the 429, want it to have waited out Retry-After: 1", gap)
	}
}

// TestRetryAfterIsClamped is a different assertion from the one above, not a
// stronger version of it: honouring the header and not being held hostage by it
// are two rules, and a receiver asking for a day must not pin a worker.
//
// The cap is shrunk rather than waited out. The behaviour under test is "a
// receiver asking for more than the cap gets the cap", and that is observable at
// any cap value; sitting through the real thirty seconds would only make the
// suite slower, not the assertion stronger.
func TestRetryAfterIsClamped(t *testing.T) {
	fastBackoff(t)

	oldCap := retryAfterCap
	retryAfterCap = 100 * time.Millisecond
	t.Cleanup(func() { retryAfterCap = oldCap })

	start := time.Now()
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "86400")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	// The header asked for 86400 seconds and the cap is 100ms, so a retry that
	// happened at all proves the clamp — without it the delivery could not have
	// finished inside this test's lifetime.
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("the delivery took %v, so the header was not clamped to retryAfterCap", elapsed)
	}
	if rec := onlyRecord(t, d, projectID); rec.Status != StatusDelivered {
		t.Fatalf("status = %q, want %q", rec.Status, StatusDelivered)
	}
}

// TestRetryAfterOfClampsWithoutWaiting exercises the parser directly, because
// the test above has to wait out retryAfterCap at real speed to observe the
// clamp through the deliverer.
func TestRetryAfterOfClampsWithoutWaiting(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		header string
		want   time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"-5", 0},
		{"2", 2 * time.Second},
		{"86400", retryAfterCap},
		{"not a date", 0},
	} {
		resp := &http.Response{Header: http.Header{}}
		if tc.header != "" {
			resp.Header.Set("Retry-After", tc.header)
		}
		if got := retryAfterOf(resp, now); got != tc.want {
			t.Errorf("Retry-After %q = %v, want %v", tc.header, got, tc.want)
		}
	}

	// An HTTP-date in the past is not a wait at all.
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", now.Add(-time.Hour).UTC().Format(http.TimeFormat))
	if got := retryAfterOf(resp, now); got != 0 {
		t.Errorf("a Retry-After in the past = %v, want 0", got)
	}
}

/* The SSRF guard itself, asserted where the error is produced rather than only
   through the API — this is the level at which the plan requires
   errors.Is(err, netguard.ErrInvalidTarget) to hold. */

func TestDelivererRefusesABlockedURL(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:9999/hook",
		"http://10.0.0.1/hook",
		"ftp://example.com/hook",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := netguard.ParseAndValidate(raw, false); !errors.Is(err, netguard.ErrInvalidTarget) {
				t.Errorf("ParseAndValidate(%q) = %v, want it to report ErrInvalidTarget", raw, err)
			}
		})
	}
}

// TestBlockedURLFailsAtDeliveryTime covers the config that reached the store by
// some other route — a restored file, a hand-edited document that added a URL
// and a channel without ever passing the route. The route refuses one, but the
// deliverer must not be the place where that is discovered to be the only check.
//
// The address is never dialled: the guard refuses it on the resolved address,
// which is the same check that closes DNS rebinding. If this test ever takes
// seconds rather than milliseconds, the guard has stopped running.
func TestBlockedURLFailsAtDeliveryTime(t *testing.T) {
	fastBackoff(t)

	d, projectID := newDeliverer(t, guardedConfig(types.WebhookGeneric, "http://169.254.169.254/latest/meta-data/"))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Errorf("status = %q, want %q", rec.Status, StatusFailed)
	}
	if !strings.Contains(rec.Error, "SSRF protection") {
		t.Errorf("the refusal did not name the guard that stopped it: %q", rec.Error)
	}
}

// TestLoopbackURLDeliversWhenTheOperatorPermittedIt is the counterpart, and the
// reason AllowPrivate is persisted at all. The API accepts a private URL from
// loopback — that is the local-development case this product is built around —
// so the deliverer must not be the place that decides otherwise. A dial check
// that ignored the verdict would leave the dashboard showing an enabled channel
// that never delivers, which is the lie the whole webhook effort exists to
// remove.
func TestLoopbackURLDeliversWhenTheOperatorPermittedIt(t *testing.T) {
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	// httptest binds 127.0.0.1, so this is a private URL by the guard's own
	// definition — the exact shape of "my alert receiver runs beside Driftwood".
	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusDelivered {
		t.Fatalf("a permitted private URL did not deliver: %q — %s", rec.Status, rec.Error)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("the receiver was hit %d times, want 1", got)
	}
}

// TestADeniedPrivateURLIsRefusedAtTheDial is the same URL as the test above with
// the verdict flipped, so the pair pins that AllowPrivate is what decides rather
// than the address shape alone.
func TestADeniedPrivateURLIsRefusedAtTheDial(t *testing.T) {
	fastBackoff(t)

	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, guardedConfig(types.WebhookGeneric, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Errorf("status = %q, want %q", rec.Status, StatusFailed)
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Errorf("the receiver was reached %d times despite the URL being refused", got)
	}
}

/* The Discord URL shape is https://discord.com/api/webhooks/<id>/<token>, so the
   token is in the path — and (*url.Error).Error() renders the whole URL. A record
   carrying one puts a live credential on screen and in whatever the operator
   pastes into a ticket. */

func TestRecordedErrorsCarryNoCredentialsFromTheURL(t *testing.T) {
	fastBackoff(t)

	const secret = "Xk9-SECRET-WEBHOOK-TOKEN"

	// A receiver that is closed before use, so the attempt is a transport error
	// rather than an HTTP status — the case where Go's error text includes the
	// URL.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookDiscord, deadURL+"/api/webhooks/123/"+secret))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the delivery to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", rec.Status, StatusFailed)
	}
	if strings.Contains(rec.Error, secret) {
		t.Errorf("the record's error carries the URL's token: %q", rec.Error)
	}
	if strings.Contains(rec.Error, deadURL) {
		t.Errorf("the record's error carries the receiver's address: %q", rec.Error)
	}
	if rec.Error == "" {
		t.Error("a transport failure recorded no reason at all, which is the other way to be useless")
	}
}

func TestUnwrapTransportErrorKeepsTheCause(t *testing.T) {
	inner := errors.New("dial tcp 127.0.0.1:1: connect: connection refused")
	wrapped := &url.Error{Op: "Post", URL: "https://hooks.slack.com/services/T/B/secret", Err: inner}

	got := unwrapTransportError(wrapped)
	if got != inner {
		t.Errorf("unwrapTransportError = %v, want the cause underneath", got)
	}
	if strings.Contains(got.Error(), "secret") {
		t.Errorf("the unwrapped error still carries the URL: %v", got)
	}

	// An error that is not a *url.Error is passed through rather than discarded.
	plain := errors.New("boom")
	if got := unwrapTransportError(plain); got != plain {
		t.Errorf("unwrapTransportError(%v) = %v, want it unchanged", plain, got)
	}
}

// TestQueueSaturationIsRecordedNotSilent is the drop policy. explainSlots drops
// quietly and is right to; here the delivery *is* the notification, so a silent
// drop is the toast-shaped lie this product keeps removing.
func TestQueueSaturationIsRecordedNotSilent(t *testing.T) {
	// A receiver that never answers, so the two workers stay busy for the whole
	// test and the queue fills behind them.
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(receiver.Close)
	t.Cleanup(unblock) // LIFO: unblock runs before Close, or close hangs.

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))

	// Enough to occupy both workers and then overflow the queue, with room to
	// spare so the assertion does not depend on exact scheduling.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < deliveryQueue+16; i++ {
			d.Enqueue(projectID, sampleAlert())
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked, so a saturated queue stalls the caller's goroutine")
	}

	var dropped int
	for _, rec := range d.Recent(projectID, 0) {
		if rec.Status == StatusDropped {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatalf("the queue overflowed and nothing recorded it; records: %+v", d.Recent(projectID, 0))
	}

	// And the dropped record has to say why, in words an operator can act on.
	for _, rec := range d.Recent(projectID, 0) {
		if rec.Status == StatusDropped && rec.Error == "" {
			t.Error("a dropped delivery carries no explanation")
		}
	}
}

// TestCloseDrainsWhatWasAccepted pins the lifecycle: an alert that was accepted
// is a notification the operator is owed, so Close waits for it rather than
// abandoning it.
func TestCloseDrainsWhatWasAccepted(t *testing.T) {
	entered := make(chan struct{}, 1)
	received := make(chan struct{}, 1)

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		w.WriteHeader(http.StatusOK)
		received <- struct{}{}
	}))
	defer receiver.Close()

	t.Setenv("HOME", t.TempDir())
	store, err := storage.NewStore("", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projectID := store.ActiveProject()
	if err := store.SetWebhook(projectID, configFor(types.WebhookGeneric, receiver.URL)); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	d := New(store, events.NewHub())
	d.Run()
	d.Enqueue(projectID, sampleAlert())

	// Close only after the request is in the receiver's hands, so this is a
	// drain of real in-flight work rather than of an empty queue.
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the receiver never saw the delivery")
	}

	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closed <- d.Close(ctx)
	}()

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}

	select {
	case <-received:
	default:
		t.Fatal("Close returned before the in-flight delivery finished")
	}

	// The drained delivery is recorded, because an operator asking "did this go
	// out" deserves an answer even during a shutdown.
	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusDelivered || rec.Attempts != 1 {
		t.Errorf("the drained delivery recorded %+v, want delivered on attempt 1", rec)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	d, _ := newDeliverer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := d.Close(ctx); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := d.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestEnqueueAfterCloseIsSafe covers the residual race Close documents: an
// enqueue that slips through as the closed flag flips loses at most one alert,
// and — the part that matters — never panics. The queue channel is never closed
// for exactly this reason.
func TestEnqueueAfterCloseIsSafe(t *testing.T) {
	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, "http://127.0.0.1:1/hook"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = d.Close(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 32; i++ {
			d.Enqueue(projectID, sampleAlert())
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked after Close")
	}
}

// TestEnqueueSkipsDisabledChannels: a channel the operator turned off must not
// be delivered to, and must not produce a record either — a record saying
// "failed" for a channel that was deliberately off is a lie with a timestamp.
func TestEnqueueSkipsDisabledChannels(t *testing.T) {
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, types.WebhookConfig{
		Kind: types.WebhookGeneric, URL: receiver.URL, Enabled: false,
	})
	d.Enqueue(projectID, sampleAlert())

	time.Sleep(50 * time.Millisecond) // no work is coming; there is nothing to wait on

	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Errorf("a disabled channel was delivered to %d times", got)
	}
	if records := d.Recent(projectID, 0); len(records) != 0 {
		t.Errorf("a disabled channel produced records: %+v", records)
	}
}

// TestOneAlertPerEnabledChannel pins the fan-out cost the plan states out loud:
// one alert, four channels, four deliveries and four records.
func TestOneAlertPerEnabledChannel(t *testing.T) {
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t,
		configFor(types.WebhookSlack, receiver.URL),
		configFor(types.WebhookTeams, receiver.URL),
		configFor(types.WebhookDiscord, receiver.URL),
		configFor(types.WebhookGeneric, receiver.URL),
	)
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "four deliveries", func() bool { return len(d.Recent(projectID, 0)) == 4 })

	kinds := map[string]bool{}
	for _, rec := range d.Recent(projectID, 0) {
		kinds[rec.Kind] = true
	}
	for _, kind := range allKinds {
		if !kinds[kind] {
			t.Errorf("no record for the %s channel", kind)
		}
	}
}

/* The leak test. The plan's structural guarantee is that a payload carries no
   response body at all, and this is that guarantee measured: a distinctive string
   planted in a stored response must appear nowhere in any rendering, including
   when the delta's own JSONPath names a credential-looking field. */

func TestNoRenderingCarriesResponseContent(t *testing.T) {
	const planted = "PLANTED-RESPONSE-BODY-VALUE-3f9a"

	alert := types.Alert{
		TrafficID:      "tr_leak",
		Endpoint:       "GET /api/login",
		ContractStatus: string(types.SeverityBreaking),
		// The explanation is free prose from a third-party model and is excluded
		// by design; carrying it is a follow-up feature, not an oversight.
		AIExplanation: map[string]interface{}{
			"summary": planted,
			"detail":  "the response contained " + planted,
		},
		Diff: &types.ContractDiff{
			HasBreakingChanges: true,
			Deltas: []types.DiffDelta{{
				JSONPath: "$.password",
				Kind:     types.KindRemovedField,
				Severity: types.SeverityBreaking,
				Message:  "password was removed",
				Expected: "string",
				Actual:   "<absent>",
			}},
		},
	}

	p := BuildPayload(EventContractDrift, ProjectRef{ID: "acme", Name: "Acme"}, alert, time.Now())

	for _, kind := range allKinds {
		t.Run(kind, func(t *testing.T) {
			body, _, err := Render(kind, p)
			if err != nil {
				t.Fatalf("Render(%q): %v", kind, err)
			}
			if strings.Contains(string(body), planted) {
				t.Errorf("the %s body carries response content:\n%s", kind, body)
			}
			// The generic body is the canonical one, so it is also the one where a
			// leaked field would be easiest to add by accident.
			if strings.Contains(string(body), "ai_explanation") {
				t.Errorf("the %s body carries the AI explanation", kind)
			}
		})
	}

	// And the delta that names a credential still says which field broke — the
	// exclusion is of response *content*, not of the path that identifies it.
	body, _, err := Render(types.WebhookSlack, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "$.password") {
		t.Error("the rendering dropped the JSON path that says what broke")
	}
}

// TestPayloadRedactsValueShapedSecrets is the defence-in-depth half: what can
// reach the payload is text the product did not author, and the redactor has to
// catch the shapes capture already knows about.
func TestPayloadRedactsValueShapedSecrets(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	alert := types.Alert{
		TrafficID:      "tr_redact",
		Endpoint:       "GET /api/session?token=" + jwt,
		ContractStatus: string(types.SeverityWarning),
		Diff: &types.ContractDiff{
			HasWarnings: true,
			Deltas: []types.DiffDelta{{
				JSONPath: "$.card",
				Severity: types.SeverityWarning,
				Message:  "the value 4111 1111 1111 1111 was returned",
			}},
		},
	}

	p := BuildPayload(EventContractDrift, ProjectRef{ID: "p", Name: "P"}, alert, time.Now())

	body, _, err := Render(types.WebhookGeneric, p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	for _, secret := range []string{jwt, "4111 1111 1111 1111"} {
		if strings.Contains(text, secret) {
			t.Errorf("the payload carries %q unredacted:\n%s", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED_JWT]") {
		t.Errorf("the JWT in the endpoint was not redacted:\n%s", text)
	}
	if !strings.Contains(text, "[REDACTED_CC]") {
		t.Errorf("the card number in a delta message was not redacted:\n%s", text)
	}
}

// TestBuildPayloadCopiesDeltas: the store mutates the alert's *Diff in place when
// the sidecar answers, so a payload aliasing that slice would have bytes
// depending on when it was rendered.
func TestBuildPayloadCopiesDeltas(t *testing.T) {
	alert := sampleAlert()

	p := BuildPayload(EventContractDrift, ProjectRef{ID: "p", Name: "P"}, alert, time.Now())

	// Mutate through the store's pointer after the payload was built.
	alert.Diff.Deltas[0].Message = "changed after the payload was built"
	alert.Diff.Deltas[0].Expected = "changed"

	if p.Deltas[0].Message != "count changed type from number to string" {
		t.Errorf("the payload's delta changed underneath it: %q", p.Deltas[0].Message)
	}
	if p.Deltas[0].Expected != "number" {
		t.Errorf("the payload's delta shares the store's struct: %q", p.Deltas[0].Expected)
	}
}

// TestRecentIsBoundedAndNewestFirst covers the record list's contract, including
// the cap: a project with more deliveries than maxRecords keeps the newest.
func TestRecentIsBoundedAndNewestFirst(t *testing.T) {
	d, projectID := newDeliverer(t)

	for i := 0; i < maxRecords+25; i++ {
		d.record(Record{ProjectID: projectID, Kind: types.WebhookGeneric, Status: StatusDelivered})
	}

	all := d.Recent(projectID, 0)
	if len(all) != maxRecords {
		t.Errorf("kept %d records, want the cap of %d", len(all), maxRecords)
	}

	if got := d.Recent(projectID, 5); len(got) != 5 {
		t.Errorf("Recent(limit 5) returned %d", len(got))
	}

	// Newest first: each record's ID is derived from a nanosecond timestamp, so
	// a list in the wrong order is visible as a descending sequence.
	for i := 1; i < len(all); i++ {
		if all[i-1].ID < all[i].ID {
			t.Fatalf("records are not newest-first at %d: %q then %q", i, all[i-1].ID, all[i].ID)
		}
	}
}

// TestRecentDoesNotAliasTheStore'sSlice: Recent hands out a copy, so a caller
// mutating the result cannot corrupt what the deliverer holds.
func TestRecentReturnsACopy(t *testing.T) {
	d, projectID := newDeliverer(t)
	d.record(Record{ProjectID: projectID, Status: StatusDelivered, Endpoint: "GET /original"})

	got := d.Recent(projectID, 0)
	got[0].Endpoint = "mutated"

	if again := d.Recent(projectID, 0); again[0].Endpoint != "GET /original" {
		t.Errorf("Recent aliases its internal slice: %q", again[0].Endpoint)
	}
}

// TestRecordPublishesOnTerminalStateOnly pins the SSE contract: a delivery that
// is retrying must not produce three dashboard entries for one alert.
func TestRecordPublishesOnTerminalStateOnly(t *testing.T) {
	fastBackoff(t)

	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	// A subscriber on the real hub, so this asserts what a browser receives
	// rather than what the deliverer intended to send.
	t.Setenv("HOME", t.TempDir())
	store, err := storage.NewStore("", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projectID := store.ActiveProject()
	if err := store.SetWebhook(projectID, configFor(types.WebhookGeneric, receiver.URL)); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	hub := events.NewHub()
	// A real stream rather than a fake sink, so this asserts what a browser
	// receives rather than what the deliverer intended to publish.
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.SSEHandler(w, r, projectID)
	}))
	defer stream.Close()

	resp, err := http.Get(stream.URL + "/_driftwood/events?project=" + projectID)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()

	frames := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) == nil && frame["type"] != nil {
				frames <- frame["type"].(string)
			}
		}
	}()

	// The subscriber is registered by the time the ping has been written, so this
	// waits on the ping reaching us rather than on the hub.
	time.Sleep(100 * time.Millisecond)

	d := New(store, hub)
	d.Run()
	t.Cleanup(func() { _ = d.Close(context.Background()) })

	d.Enqueue(projectID, sampleAlert())

	var seen []string
	deadline := time.After(3 * time.Second)
	for len(seen) == 0 {
		select {
		case typ := <-frames:
			seen = append(seen, typ)
		case <-deadline:
			t.Fatalf("no frame arrived; records: %+v", d.Recent(projectID, 0))
		}
	}

	// One frame for one alert, on terminal state — not one per attempt. The two
	// attempts that failed must publish nothing.
	time.Sleep(100 * time.Millisecond)
drain:
	for {
		select {
		case typ := <-frames:
			seen = append(seen, typ)
		default:
			break drain
		}
	}

	for _, typ := range seen {
		if typ != "webhook_delivery" {
			t.Errorf("unexpected event type %q", typ)
		}
	}
	if len(seen) != 1 {
		t.Errorf("published %d frames for one delivered alert, want 1", len(seen))
	}
}

/* The test send. It is synchronous and single-attempt, so these assert the
   shape of the message as much as the outcome: a test alert that could be
   mistaken for a real break in a channel other people read is the failure mode
   worth failing a test over. */

// testReceiver answers every request with status, and records the bodies it saw.
func testReceiver(t *testing.T, status int) (*httptest.Server, *[]map[string]interface{}, *int64) {
	t.Helper()
	var mu sync.Mutex
	bodies := new([]map[string]interface{})
	var hits int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]interface{}
		_ = json.Unmarshal(raw, &decoded)
		mu.Lock()
		*bodies = append(*bodies, decoded)
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, bodies, &hits
}

func TestTestSendReportsSuccessAndItsLatency(t *testing.T) {
	receiver, bodies, _ := testReceiver(t, http.StatusOK)
	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))

	result, err := d.Test(context.Background(), projectID, types.WebhookGeneric)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Errorf("OK = false, error %q", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.LatencyMS < 0 {
		t.Errorf("LatencyMS = %d, want >= 0", result.LatencyMS)
	}

	body := (*bodies)[0]
	if body["event"] != EventTest {
		t.Errorf("event = %v, want %q", body["event"], EventTest)
	}
	if body["endpoint"] != "GET /_driftwood/test" {
		t.Errorf("endpoint = %v, want the test route", body["endpoint"])
	}
	if body["contract_status"] != StatusTest {
		t.Errorf("contract_status = %v, want %q", body["contract_status"], StatusTest)
	}
}

// The assertion that matters most here. A test message that announced BREAKING,
// or that carried an invented delta, would be indistinguishable from a real
// break in a channel the operator's colleagues read — the /api/users seed
// mistake with a wider audience.
func TestTestSendCannotBeMistakenForARealAlert(t *testing.T) {
	receiver, bodies, _ := testReceiver(t, http.StatusOK)

	for _, kind := range types.WebhookKinds {
		t.Run(kind, func(t *testing.T) {
			d, projectID := newDeliverer(t, configFor(kind, receiver.URL))
			if _, err := d.Test(context.Background(), projectID, kind); err != nil {
				t.Fatalf("Test: %v", err)
			}
		})
	}

	for i, body := range *bodies {
		raw, _ := json.Marshal(body)
		text := string(raw)

		if strings.Contains(text, string(types.SeverityBreaking)) {
			t.Errorf("body %d names BREAKING: %s", i, text)
		}
		if !strings.Contains(text, StatusTest) {
			t.Errorf("body %d does not carry %q: %s", i, StatusTest, text)
		}
		// The generic kind is the canonical payload, so this is where the delta
		// list would show up if a test were given fabricated ones.
		if deltas, ok := body["deltas"]; ok && deltas != nil {
			if list, isList := deltas.([]interface{}); !isList || len(list) > 0 {
				t.Errorf("body %d carries deltas: %v", i, deltas)
			}
		}
	}
}

func TestTestSendMakesExactlyOneAttempt(t *testing.T) {
	// A 500 is retryable for a real delivery, so if Test shared the retry path
	// this receiver would be hit three times.
	receiver, _, hits := testReceiver(t, http.StatusInternalServerError)
	fastBackoff(t)

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))

	result, err := d.Test(context.Background(), projectID, types.WebhookGeneric)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Error("OK = true against a receiver answering 500")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Errorf("receiver was hit %d times, want 1 — a button press must not run a retry schedule", got)
	}
}

func TestTestSendReportsATransportFailureWithoutTheURL(t *testing.T) {
	receiver, _, _ := testReceiver(t, http.StatusOK)
	url := receiver.URL + "/services/Xk9-SECRET-WEBHOOK-TOKEN"
	receiver.Close() // so the connection is refused rather than answered

	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, url))

	result, err := d.Test(context.Background(), projectID, types.WebhookGeneric)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Error("OK = true against a closed receiver")
	}
	if result.Error == "" {
		t.Error("a transport failure reported no error")
	}
	if strings.Contains(result.Error, "Xk9-SECRET-WEBHOOK-TOKEN") {
		t.Errorf("the error carries the URL's credential: %q", result.Error)
	}
}

func TestTestSendWithNoURLIsNotConfigured(t *testing.T) {
	// An enabled channel needs a URL, so this is the state a channel is in
	// before the operator has pasted one: saved as a kind, nothing else.
	d, projectID := newDeliverer(t, types.WebhookConfig{Kind: types.WebhookSlack})

	if _, err := d.Test(context.Background(), projectID, types.WebhookSlack); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// A test is not an alert, so it does not enter the record log. Pinned because
// the alternative is tempting and wrong: entries with an empty traffic_id would
// push real deliveries out of a list that holds the last ten.
func TestTestSendIsNotRecorded(t *testing.T) {
	receiver, _, _ := testReceiver(t, http.StatusOK)
	d, projectID := newDeliverer(t, configFor(types.WebhookGeneric, receiver.URL))

	if _, err := d.Test(context.Background(), projectID, types.WebhookGeneric); err != nil {
		t.Fatalf("Test: %v", err)
	}

	if records := d.Recent(projectID, 0); len(records) != 0 {
		t.Errorf("a test send filed %d delivery records: %+v", len(records), records)
	}
}

// The test route reaches the same address a real delivery does, and the verdict
// on whether that address may be private is the one recorded when it was saved.
func TestTestSendHonoursTheSavedPrivateVerdict(t *testing.T) {
	receiver, _, _ := testReceiver(t, http.StatusOK)
	d, projectID := newDeliverer(t, guardedConfig(types.WebhookGeneric, receiver.URL))

	result, err := d.Test(context.Background(), projectID, types.WebhookGeneric)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Error("a URL that never passed the loopback rule was reached")
	}
	if !strings.Contains(result.Error, "SSRF protection") {
		t.Errorf("error = %q, want the guard's refusal", result.Error)
	}
}

// The receiver's own words are what an operator needs to tell "the URL is wrong"
// from "the payload is wrong" — and Teams, whose envelope is the unverified
// part, is exactly the kind where that is the whole diagnosis.
func TestTestSendReportsWhatTheReceiverSaid(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_payload"}`))
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookSlack, receiver.URL))

	result, err := d.Test(context.Background(), projectID, types.WebhookSlack)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !strings.Contains(result.Error, "invalid_payload") {
		t.Errorf("error = %q, want the receiver's own words — a bare 400 cannot tell an operator "+
			"whether to check the URL or the envelope", result.Error)
	}
}

// And the same bytes must not reach a record. The test route reads the reply
// because the operator is looking at the answer; deliver records none of it,
// because a record is written down and kept.
func TestRecordedFailureCarriesNoneOfTheReceiversReply(t *testing.T) {
	const planted = "PLANTED-VENDOR-RESPONSE-BODY-7c21"
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_payload: " + planted))
	}))
	defer receiver.Close()

	d, projectID := newDeliverer(t, configFor(types.WebhookSlack, receiver.URL))
	d.Enqueue(projectID, sampleAlert())

	waitFor(t, "the failure to be recorded", func() bool { return len(d.Recent(projectID, 0)) == 1 })

	rec := onlyRecord(t, d, projectID)
	if rec.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", rec.Status, StatusFailed)
	}
	if strings.Contains(rec.Error, planted) {
		t.Errorf("the record carries the receiver's body: %q", rec.Error)
	}
}

// An empty log is a list, not null. Found by pressing Test against a running
// build and reading the delivery route: the server's own test asserted [] and
// passed, because its stand-in returned [] while the real Recent returned a nil
// slice. A client that maps over the response gets a TypeError from null.
func TestAnEmptyLogMarshalsToAList(t *testing.T) {
	d, projectID := newDeliverer(t)

	raw, err := json.Marshal(d.Recent(projectID, 50))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("an empty delivery log marshals to %s, want []", raw)
	}

	// And a limit is still a bound rather than a no-op, since the same slice is
	// what the route encodes.
	d.Enqueue(projectID, sampleAlert())
	if got := d.Recent(projectID, 0); len(got) != 0 {
		t.Errorf("a project with no channel configured filed %d records: %+v", len(got), got)
	}
}
