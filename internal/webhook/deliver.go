package webhook

/* Getting an alert off the machine.

   Driftwood detects contract drift and shows it in one browser tab. Nothing left
   that tab, so a team not looking at it learned nothing, and that is the last
   thing standing between the product and its stated purpose: drift is detected,
   and then it goes nowhere.

   This is the first real background worker in the repo and it should be named as
   such, because the two that came before it are not models to copy. explainAsync
   has no lifecycle at all — no logging, no WaitGroup, no drain, every failure a
   bare return. events.Hub's `go h.run()` never exits. So the shape here is
   explicit: Run starts workers, Close stops accepting, drains what was already
   accepted, and waits under the caller's context.

   Records live here rather than in the store, and are not persisted. Alerts are
   in-memory only, so a record has the same lifetime as the thing it describes;
   and historiesForPersistLocked exists precisely to keep runtime telemetry out
   of the persisted document, which otherwise holds nothing but accepted
   contracts. Keeping them out also keeps the v3 migration a config migration
   only. */

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/donaina/driftwood/internal/capture"
	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/netguard"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

// The constants below are reasoned, not measured. They are in one block with
// their reasoning, so the first real deployment can tune them from evidence
// rather than from a guess scattered across the file.
const (
	// deliveryWorkers is deliberately not explainBudget's four. Retries happen
	// *inside* a worker, so a dead endpoint holds a worker for the whole retry
	// schedule — one worker means one broken Slack URL stalls another project's
	// deliveries. Four was tuned for a local sidecar; this egress is arbitrary
	// internet hosts, which are slower and likelier to be down.
	deliveryWorkers = 2

	// deliveryQueue bounds how many deliveries may be waiting. Past it an alert
	// is dropped and a record says so — see Enqueue.
	deliveryQueue = 64

	// deliveryTimeout matches explainAsync's client, which is the only other
	// outbound call this product makes on a timer.
	deliveryTimeout = 8 * time.Second

	// maxDeliveryAttempts counts the first try. Worst case a worker is held for
	// the sum of backoffSchedule plus three timeouts, which bounds both the
	// schedule and how fast the queue can drain.
	maxDeliveryAttempts = 3

	// maxRecords is maxAlerts, so a reader who knows one number knows the other.
	// Per project, and evicted newest-first by dropping the oldest.
	maxRecords = 200
)

// retryAfterCap bounds how long a receiver's Retry-After can hold a worker.
//
// Not optional, and not a const, for two unrelated reasons. The cap itself is
// not optional because the header is the receiver's to choose, and a hostile or
// merely broken one sending "Retry-After: 86400" would otherwise pin a worker
// for a day. It is a var for the same reason backoffSchedule is — a test that
// wants to observe the clamp would otherwise have to wait out the real thirty
// seconds, and a test nobody can afford to run is a test that stops being run.
var retryAfterCap = 30 * time.Second

// backoffSchedule is the wait before attempt 2 and before attempt 3.
//
// A package-level var rather than a const only so a test can shrink it, which is
// the explainURL convention — this codebase has no clock abstraction (no nowFunc,
// no injected timer, zero production time.Sleep) and introducing a Clock
// interface for one caller would be a larger idea than the problem.
var backoffSchedule = []time.Duration{time.Second, 4 * time.Second}

// Delivery outcomes, as a record's Status.
const (
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	// StatusDropped is the queue-saturation outcome. It is a record rather than
	// a silence because here the delivery *is* the notification: explainSlots
	// drops quietly and is right to, since a paragraph about an alert you are
	// already reading is worth less than a fast one, but a dropped alert is the
	// toast-shaped lie in a new costume.
	StatusDropped = "dropped"
)

// Sink receives alerts raised for a project.
//
// It lives here rather than in pkg/types so that internal/proxy can depend on
// this package without this package depending on internal/proxy — the proxy
// hands alerts to the deliverer, and the deliverer needs the SSRF guards, which
// is why those moved to netguard in the first place.
type Sink interface {
	Enqueue(projectID string, alert types.Alert)
}

// Discard is the default Sink, so a proxy whose delivery was never wired is a
// visible no-op rather than a nil dereference.
//
// This is the explainSlots lesson stated as a type. A nil channel in a
// hand-built test proxy silently dropped every explanation in exactly the tests
// meant to catch that; a defaulted Discard makes a forgotten wiring do nothing
// loudly instead of nothing invisibly.
type Discard struct{}

func (Discard) Enqueue(string, types.Alert) {}

// Deliveries is what the API needs from the deliverer: the records it has filed,
// and the ability to send one test message.
//
// It was called DeliveryLog, and the name stopped being true when the test route
// arrived. The server no longer only reads — it can cause an outbound request —
// and a name describing the old, narrower thing is how the next reader comes to
// believe the enqueue side is still out of reach from here. It is not: Test is a
// send, deliberately synchronous and deliberately not Enqueue.
type Deliveries interface {
	Recent(projectID string, limit int) []Record
	Test(ctx context.Context, projectID, kind string) (TestResult, error)
}

// NoDeliveries is a deliverer that holds nothing and can send nothing, for a
// server built without one.
type NoDeliveries struct{}

func (NoDeliveries) Recent(string, int) []Record { return []Record{} }

// Test fails rather than reporting a result, and that is the honest answer: a
// server with no deliverer did not reach a receiver and was not refused by one,
// so it has nothing to report about a channel that nothing is wired to deliver
// to. It is not ErrNotConfigured — nothing about the request was wrong — so the
// route answers 500 rather than telling the operator to check a URL that is
// already fine.
func (NoDeliveries) Test(context.Context, string, string) (TestResult, error) {
	return TestResult{}, errors.New("no alert deliverer is configured")
}

// ErrNotConfigured is Test's only error, and it means "there is nothing to
// test" rather than "the test failed".
//
// The split is the ErrInvalidTarget idiom applied to a button: a channel with no
// URL is the caller's problem and answers 400, while a receiver that answered
// 400 is a result — the whole point of the route is to report that rather than
// to fail the request that asked for it.
var ErrNotConfigured = errors.New("webhook is not configured")

// Record is one attempt-series against one channel: what was tried, how it went,
// and how many times it was tried.
type Record struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	TrafficID  string    `json:"traffic_id"`
	Endpoint   string    `json:"endpoint"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Attempts   int       `json:"attempts"`
	StatusCode int       `json:"status_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	// CompletedAt is zero while a delivery is still in flight, which is how the
	// dashboard can tell "no record yet" from "failed".
	CompletedAt time.Time `json:"completed_at"`
}

// Deliverer posts alerts to the channels an operator configured.
type Deliverer struct {
	store *storage.Store
	hub   *events.Hub

	// Two clients, one decision. A config carries the verdict on whether its URL
	// may be private — see types.WebhookConfig.AllowPrivate — and the transport's
	// dial has to express that same verdict, because the guards judge a URL
	// string and the resolver is consulted again at connection time. One client
	// cannot hold both answers, so the choice is made per job in post.
	client        *http.Client
	privateClient *http.Client

	queue chan job

	// closing is the drain signal, and ctx is the abort. They are two channels
	// rather than one because they mean different things and Close needs both.
	//
	// Closing a single context would do both at once: cancelling d.ctx aborts any
	// request currently in flight, so "drain what was accepted" would in practice
	// mean "kill every accepted delivery and file each as abandoned" — worse than
	// abandoning them, because the record log would fill with failures that never
	// happened. So closing says stop accepting and finish what you have; ctx stays
	// alive until either the work is done or the caller's budget expires, and only
	// then aborts.
	closing chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc

	mu      sync.Mutex
	records map[string][]Record
	closed  bool

	wg sync.WaitGroup
}

// job is one alert headed for one channel. Per channel rather than per alert, so
// that retrying a dead Discord URL does not delay the Slack delivery that would
// have succeeded, and so each channel gets its own record.
type job struct {
	projectID string
	kind      string
	alert     types.Alert
}

// New builds a deliverer. Call Run to start it.
func New(store *storage.Store, hub *events.Hub) *Deliverer {
	ctx, cancel := context.WithCancel(context.Background())

	return &Deliverer{
		store:         store,
		hub:           hub,
		ctx:           ctx,
		cancel:        cancel,
		client:        deliveryClient(false),
		privateClient: deliveryClient(true),
		queue:         make(chan job, deliveryQueue),
		closing:       make(chan struct{}),
		records:       make(map[string][]Record),
	}
}

// deliveryClient builds one of the two delivery clients — guarded, or permitting
// a private address because the operator named one from loopback.
func deliveryClient(allowPrivate bool) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// The guards judge the URL string once, at save time. The resolver is
		// consulted again here, and a name whose authoritative server answers
		// differently the second time — DNS rebinding, TTL 0 — would reach
		// loopback with every string check passed. DialContext refuses a
		// connection whose resolved address is blocked and then dials that
		// address rather than the name.
		DialContext:           netguard.DialContext(&net.Dialer{Timeout: 5 * time.Second}, allowPrivate),
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: deliveryTimeout,
		MaxIdleConns:          deliveryWorkers,
		IdleConnTimeout:       30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   deliveryTimeout,
		// A 3xx must be terminal and must not be followed. The guards check
		// the configured URL's host once; Go's default client would then
		// follow up to ten redirects, so a webhook at an operator-named
		// public host that answers "302 Location: http://169.254.169.254/"
		// would reach link-local with every check passed. Returning
		// ErrUseLastResponse turns the hop into a response this code can see
		// and refuse, rather than a request Driftwood makes.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Run starts the workers. It returns immediately.
func (d *Deliverer) Run() {
	for i := 0; i < deliveryWorkers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
}

// Close stops accepting new deliveries, drains what was already queued, and
// waits for the workers.
//
// Draining rather than abandoning is deliberate: an alert that was accepted is a
// notification the operator is owed, and one already in flight costs a bounded
// wait to honour. ctx is the caller's budget for that — a receiver that has
// stopped answering must not hold shutdown open for its whole retry schedule, so
// when the budget expires the requests are aborted and Close says what it left.
func (d *Deliverer) Close(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()

	// Tells the workers to finish what is queued and then return. It does not
	// touch d.ctx — see the field comment for why that distinction is the whole
	// design of this function.
	close(d.closing)

	finished := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
		// Nothing is in flight, so this only releases the request contexts.
		d.cancel()
		return nil
	case <-ctx.Done():
		// Budget spent. This is what d.ctx exists for: an in-flight request
		// against a receiver that has stopped answering would otherwise hold
		// Close — and the process — for the rest of its retry schedule.
		d.cancel()
		// The workers now have nothing left to block on: an in-flight request
		// returns, a backoff wait returns, and the drain loop stops. Waiting here
		// is what makes Close's return mean the workers have actually stopped.
		<-finished
		return fmt.Errorf("alert deliveries still draining: %w", ctx.Err())
	}
}

// Enqueue hands an alert to the delivery workers. It never blocks.
//
// The queue channel is never closed, deliberately. Enqueue is called from the
// request path and httpServer.Shutdown may still be draining in-flight handlers
// when Close runs; a send on a closed channel is a panic, and a panic in the
// request path is the worst possible outcome of a clean shutdown. The residual
// race — an enqueue that slips through as the closed flag flips — loses at most
// one alert.
func (d *Deliverer) Enqueue(projectID string, alert types.Alert) {
	// One read per alert, on the request path, of configs already in memory.
	// AddTraffic takes the store's write lock on this same path, so this is not
	// a new class of blocking.
	configs := d.store.GetWebhooks(projectID)

	for _, cfg := range configs {
		if !cfg.Enabled || cfg.URL == "" {
			continue
		}

		// Already stopped, or stopping. Either way no worker will pick this up,
		// and a send would sit in the buffered queue until the process exits —
		// an alert that vanished with nothing said about it, which is the one
		// outcome this whole path exists to avoid.
		select {
		case <-d.closing:
			d.recordDropped(projectID, alert, cfg.Kind, "Driftwood is shutting down; this alert was not sent")
			continue
		default:
		}

		select {
		case d.queue <- job{projectID: projectID, kind: cfg.Kind, alert: alert}:
		default:
			// Saturated. Drop it, and say so — see StatusDropped.
			d.recordDropped(projectID, alert, cfg.Kind, "the delivery queue was full; this alert was not sent")
		}
	}
}

// recordDropped files an alert that was accepted for delivery and then not sent.
//
// A record rather than a silence: here the delivery *is* the notification, so a
// drop the dashboard cannot show is indistinguishable from an alert that was
// never raised.
func (d *Deliverer) recordDropped(projectID string, alert types.Alert, kind, reason string) {
	now := time.Now()
	d.record(Record{
		ProjectID:   projectID,
		TrafficID:   alert.TrafficID,
		Endpoint:    alert.Endpoint,
		Kind:        kind,
		Status:      StatusDropped,
		Error:       reason,
		DetectedAt:  now,
		CompletedAt: now,
	})
}

func (d *Deliverer) worker() {
	defer d.wg.Done()

	for {
		select {
		case j := <-d.queue:
			d.deliver(j)
		case <-d.closing:
			// Drain what is already queued, then leave. Not accepting new work is
			// what closing means; abandoning accepted work is not.
			for {
				select {
				case j := <-d.queue:
					d.deliver(j)
				case <-d.ctx.Done():
					// The caller's drain budget expired. Stop rather than keep
					// filing failures: every remaining attempt would fail
					// instantly on a cancelled context and be recorded as a
					// broken receiver, which is a record of something that never
					// happened.
					return
				default:
					return
				}
			}
		}
	}
}

// deliver runs one job's attempt series and records the outcome.
func (d *Deliverer) deliver(j job) {
	cfg, ok := d.store.GetWebhook(j.projectID, j.kind)
	if !ok || !cfg.Enabled || cfg.URL == "" {
		// Disabled or removed between the enqueue and the attempt. There is
		// nothing to deliver to and nothing went wrong, so no record: a record
		// saying "failed" for a channel the operator deliberately turned off
		// would be a lie with a timestamp on it.
		return
	}

	payload := BuildPayload(EventContractDrift, d.projectRef(j.projectID), j.alert, time.Now())
	body, contentType, err := Render(j.kind, payload)
	if err != nil {
		d.recordFailed(j, 0, 0, err, time.Now())
		return
	}

	detectedAt := time.Now()
	attempts := 0
	var lastStatus int
	var lastErr error

	for {
		attempts++

		// out.reply is not kept. What a vendor says about a refused delivery is
		// the vendor's bytes, and a record says what we tried and what came back
		// at the level an operator acts on: the status code and our own text.
		// Test is where the reply is shown, because there the operator is looking
		// at the answer and nothing is being written down.
		out := d.post(d.ctx, cfg, contentType, body)
		status, retryAfter, err := out.status, out.retryAfter, out.err
		lastStatus, lastErr = status, err

		if err == nil && status >= 200 && status < 300 {
			d.record(Record{
				ProjectID:   j.projectID,
				TrafficID:   j.alert.TrafficID,
				Endpoint:    j.alert.Endpoint,
				Kind:        j.kind,
				Status:      StatusDelivered,
				Attempts:    attempts,
				StatusCode:  status,
				DetectedAt:  detectedAt,
				CompletedAt: time.Now(),
			})
			return
		}

		if !retryable(status, err) || attempts >= maxDeliveryAttempts {
			break
		}

		wait := backoffFor(attempts)
		if retryAfter > 0 {
			wait = retryAfter
		}
		if !d.sleep(wait) {
			// Shutting down, or already shut down. Recorded, because an operator
			// asking "did this go out" deserves an answer even when the answer is
			// "we stopped trying".
			lastErr = errShuttingDown
			break
		}
	}

	d.recordFailed(j, attempts, lastStatus, lastErr, detectedAt)
}

// TestResult is what one test send did, and it is the whole response body.
//
// StatusCode is zero when the attempt never got a response — a refused
// connection, a DNS failure, a timeout. Error is our own text in that case and
// the receiver's status in the other; there is deliberately no field for what
// the receiver *said*, for the reason post drains rather than reads the body.
type TestResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

// Test sends one sample alert to a configured channel and reports what happened.
//
// Synchronous because it is not on the proxy path and nobody is waiting on
// anything else: the operator pressed a button, and the answer is the response.
// One attempt, no retry — a retry schedule behind a button turns a clear
// "connection refused" into a fifteen-second wait that ends in the same answer,
// and the operator can press it again.
//
// It sends whether or not the channel is enabled. Configuring a URL, testing it
// and then turning the channel on is the order an operator actually works in,
// and a Test button that refuses until the toggle is flipped would be a control
// whose effect depends on another control.
//
// The send is not recorded. A record answers "did my alert get delivered", and a
// test is not an alert: entries with an empty traffic_id and a made-up endpoint
// would push real deliveries out of a list that holds the last ten. The card
// reports the outcome instead, in the response the operator is already looking
// at.
func (d *Deliverer) Test(ctx context.Context, projectID, kind string) (TestResult, error) {
	cfg, ok := d.store.GetWebhook(projectID, kind)
	if !ok || cfg.URL == "" {
		return TestResult{}, fmt.Errorf("no URL is saved for the %s channel: %w", kind, ErrNotConfigured)
	}

	body, contentType, err := Render(kind, BuildPayload(EventTest, d.projectRef(projectID), testAlert(), time.Now()))
	if err != nil {
		return TestResult{}, err
	}

	start := time.Now()
	out := d.post(ctx, cfg, contentType, body)
	latency := time.Since(start)

	result := TestResult{StatusCode: out.status, LatencyMS: latency.Milliseconds()}
	switch {
	case out.err != nil:
		// Unwrapped already by post, then redacted because this text reaches a
		// screen and bounded because it reaches a browser.
		result.Error = truncate(capture.RedactValue(out.err.Error()), 300)
	case out.status >= 200 && out.status < 300:
		result.OK = true
	case out.reply != "":
		// The receiver's own words, redacted and bounded by post. This is the
		// whole diagnosis when a payload is wrong rather than a URL: "400" alone
		// cannot tell an operator whether to check the URL or the envelope, and
		// Teams is the kind where the envelope is the thing most likely to be
		// wrong.
		result.Error = out.reply
	default:
		result.Error = "the receiver refused the test message and said nothing about why"
	}
	return result, nil
}

// StatusTest is the contract_status a test send carries.
//
// Deliberately not one of the three severities. A test message that said
// BREAKING would be indistinguishable from a real break in a channel other
// people read, and would go on saying so in the scrollback after the operator
// has forgotten they pressed a button.
const StatusTest = "TEST"

// testAlert is the alert a test send is built from: no deltas, and a status that
// is not a severity.
//
// Both halves are one decision. This codebase has a scar about fabricating a
// contract that looks real — the deleted /api/users seed that made the first
// healthy response look BREAKING — and inventing a delta to make the test
// message look more like a real one would be that mistake with a wider audience.
// So there is nothing to invent one from, the endpoint names a route Driftwood
// serves itself, and the event field says driftwood_test to anything parsing the
// body rather than reading it.
func testAlert() types.Alert {
	return types.Alert{
		Endpoint:       "GET /_driftwood/test",
		ContractStatus: StatusTest,
	}
}

// maxReplyBytes bounds how much of a receiver's reply is kept. Two kilobytes is
// several times the longest vendor error body and remains useful for the HTML a
// proxy in front of a webhook returns when it refuses one.
const maxReplyBytes = 2048

// attempt is one POST's outcome, in full.
//
// reply is the receiver's own words, bounded and redacted, and it is read by
// exactly one caller: Test, which reports it on the card. deliver deliberately
// does not record it — see recordFailed — and the field is named here rather
// than returned positionally so that the choice to ignore it is visible at the
// place that makes it.
//
// Reading it at all is what lets an operator tell "the URL is wrong" from "the
// payload is wrong". A bare 400 cannot distinguish them, and the Teams envelope
// is the case where that distinction is the entire diagnosis: Power Automate
// answers a wrong envelope with a 400 and a sentence saying so.
type attempt struct {
	status     int
	retryAfter time.Duration
	reply      string
	err        error
}

// post makes one attempt. It returns the status code, a Retry-After duration if
// the receiver sent one, the receiver's reply, and the transport error if the
// attempt never got a response.
//
// The client is chosen from the config rather than fixed on the Deliverer: the
// config is what carries the operator's answer about private addresses, and the
// dial has to honour it or a URL the API accepted is a URL every delivery
// refuses.
//
// ctx is a parameter rather than d.ctx because there are two callers with
// different lifetimes. A worker's delivery is bounded by the deliverer's own
// context, which outlives any one request. A test send is on a request, and if
// the operator closes the tab mid-test the request that asked for it is gone —
// there is nobody left to report the answer to, so the send should stop with it.
func (d *Deliverer) post(ctx context.Context, cfg types.WebhookConfig, contentType string, body []byte) attempt {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return attempt{err: err}
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Driftwood")

	client := d.client
	if cfg.AllowPrivate {
		client = d.privateClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return attempt{err: unwrapTransportError(err)}
	}
	// Read a bounded prefix, then drain the rest. Draining is not optional: a
	// response left unread keeps the connection out of the pool.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return attempt{
		status:     resp.StatusCode,
		retryAfter: retryAfterOf(resp, time.Now()),
		reply:      truncate(capture.RedactValue(strings.TrimSpace(string(raw))), maxReplyBytes),
	}
}

// retryable reports whether another attempt could plausibly succeed.
func retryable(status int, err error) bool {
	if err != nil {
		// A transport failure is worth another go. A cancelled context is not:
		// the process is going down, and d.deliver records that separately.
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
	}
	switch {
	case status == http.StatusRequestTimeout: // 408
		return true
	case status == http.StatusTooManyRequests: // 429
		return true
	case status >= 500:
		return true
	case status >= 300 && status < 400:
		// A redirect is terminal, never retried. Following it is the SSRF bypass
		// CheckRedirect exists to refuse; retrying it would just repeat a request
		// that will answer the same way.
		return false
	default:
		// Any other 4xx. A bad payload or a revoked URL does not fix itself, and
		// three attempts turn a clear 400 into a slow one.
		return false
	}
}

func backoffFor(attempt int) time.Duration {
	if attempt-1 < len(backoffSchedule) {
		return backoffSchedule[attempt-1]
	}
	return backoffSchedule[len(backoffSchedule)-1]
}

// sleep waits out a backoff, returning false if another attempt should not be
// made.
//
// Both stop conditions return false, and they mean different things: closing
// says there is no time left for another schedule, ctx says the caller's budget
// is gone. Either way the delivery is recorded as abandoned rather than as a
// receiver that failed.
func (d *Deliverer) sleep(dur time.Duration) bool {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-d.closing:
		return false
	case <-d.ctx.Done():
		return false
	}
}

// retryAfterOf reads Retry-After, clamped.
//
// Honouring the header and not being held hostage by it are two different
// assertions, so both live here: a receiver asking for a second gets one, and a
// receiver asking for a day gets retryAfterCap.
func retryAfterOf(resp *http.Response, now time.Time) time.Duration {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}

	var d time.Duration
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return 0
		}
		d = time.Duration(secs) * time.Second
	} else if when, err := http.ParseTime(raw); err == nil {
		d = when.Sub(now)
	} else {
		return 0
	}

	if d <= 0 {
		return 0
	}
	if d > retryAfterCap {
		return retryAfterCap
	}
	return d
}

// unwrapTransportError strips the *url.Error wrapper, which embeds the whole URL.
//
// These URLs carry credentials in the path — https://discord.com/api/webhooks/
// <id>/<token> is the normal Discord shape — and (*url.Error).Error() renders as
// `Post "https://discord.com/api/webhooks/…": dial tcp: …`. Recording that puts
// a live token in a delivery record, on screen, and in whatever the operator
// pastes into a ticket. The cause underneath is the part worth keeping.
func unwrapTransportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err
	}
	return err
}

// projectRef names a project the way a channel reader would recognise it.
func (d *Deliverer) projectRef(projectID string) ProjectRef {
	ref := ProjectRef{ID: projectID, Name: projectID}
	if p, ok := d.store.GetProject(projectID); ok && p.Name != "" {
		ref.Name = p.Name
	}
	return ref
}

// errShuttingDown is the terminal reason for a delivery that was abandoned
// because the process is stopping. Distinct from a receiver failure so the
// record does not read as though the channel is broken.
var errShuttingDown = errors.New("delivery abandoned: Driftwood is shutting down")

// recordFailed files a terminal failure, with the error text unwrapped, redacted
// and bounded.
func (d *Deliverer) recordFailed(j job, attempts, status int, err error, detectedAt time.Time) {
	msg := "the receiver refused the payload"
	switch {
	case err == nil:
		// A terminal status with no transport error: 4xx or 3xx. The status code
		// on the record is the whole story.
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, errShuttingDown):
		msg = errShuttingDown.Error()
	default:
		// Redacted because this text reaches a screen, and bounded because it
		// reaches a file. The URL itself is already gone — see
		// unwrapTransportError.
		msg = truncate(capture.RedactValue(err.Error()), 300)
	}
	d.record(Record{
		ProjectID:   j.projectID,
		TrafficID:   j.alert.TrafficID,
		Endpoint:    j.alert.Endpoint,
		Kind:        j.kind,
		Status:      StatusFailed,
		Attempts:    attempts,
		StatusCode:  status,
		Error:       msg,
		DetectedAt:  detectedAt,
		CompletedAt: time.Now(),
	})
}

// record files a terminal outcome and announces it.
//
// The event fires on terminal state only, so a delivery that is retrying does
// not produce three dashboard entries for one alert.
func (d *Deliverer) record(r Record) {
	r.ID = fmt.Sprintf("wd_%d", time.Now().UnixNano())

	d.mu.Lock()
	list := append([]Record{r}, d.records[r.ProjectID]...)
	if len(list) > maxRecords {
		// Copy the tail rather than reslicing forward, for the reason spelled
		// out in AddTraffic's alert eviction: `list[:maxRecords]` would keep the
		// whole backing array alive and it would never grow back down.
		list = append([]Record(nil), list[:maxRecords]...)
	}
	d.records[r.ProjectID] = list
	d.mu.Unlock()

	d.hub.Publish(r.ProjectID, "webhook_delivery", r)
}

// Recent returns a project's delivery records, newest first.
//
// The copy is built onto a non-nil empty slice, so a project with no deliveries
// answers [] rather than null. That is not a detail of encoding: the API's
// published contract for this route is a list, and a client that does
// `records.map(...)` gets a TypeError from null. It shipped as null once, and
// the server's own test did not catch it because the test's stand-in returned
// [] — which is the whole hazard of asserting against a stand-in.
func (d *Deliverer) Recent(projectID string, limit int) []Record {
	d.mu.Lock()
	defer d.mu.Unlock()

	list := d.records[projectID]
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return append([]Record{}, list...)
}
