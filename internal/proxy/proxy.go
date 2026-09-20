package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/donaina/driftwood/internal/capture"
	"github.com/donaina/driftwood/internal/diff"
	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

// The reserved namespace. Driftwood's own control plane and dashboard live under
// ControlPrefix; every other path on the port is proxied to the target API. That
// inversion is the product — a proxy that only forwards a hardcoded `/api/`
// prefix never sees the endpoints it is supposed to be watching.
//
// These live here, rather than in server, because the router and the proxy must
// agree on the boundary: the router decides what reaches the proxy, and the proxy
// decides whether a control-path request is a mock fixture or a real backend call.
// Two copies of this string would drift.
const (
	ControlPrefix = "/_driftwood"
	MockPrefix    = ControlPrefix + "/mock"
)

// inNamespace reports whether path is prefix itself or sits beneath it. The
// boundary matters: "/_driftwoodfoo" is a target-API path and must be proxied,
// so a bare strings.HasPrefix would swallow it.
func inNamespace(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// IsControlPath reports whether path belongs to Driftwood rather than the target
// API. Everything for which this is false gets proxied.
func IsControlPath(path string) bool { return inNamespace(path, ControlPrefix) }

// IsMockPath reports whether path addresses a mock fixture endpoint.
func IsMockPath(path string) bool { return inNamespace(path, MockPrefix) }

// The AI explainer is an optional sidecar: a separate Node service that turns a
// structural delta into prose. Driftwood is complete without it — an absent or
// unreachable sidecar costs one refused connection and a missing paragraph, and
// nothing else in the pipeline waits on it.
const (
	// explainBudget bounds how many sidecar calls may be in flight at once.
	//
	// A contract that has broken stays broken: every later request to that
	// endpoint is another BREAKING change and another explanation of the same
	// deltas. Left unbounded, one broken endpoint under load fans out a goroutine
	// per request, each holding a connection to the sidecar for up to its 8s
	// timeout — so the detector's own alerting becomes the thing that takes the
	// sidecar down. Past the budget the explanation is dropped rather than
	// queued, the same choice the SSE hub makes for a slow client: a paragraph
	// about an alert the user is already reading is worth less than a fast one,
	// and the alert itself never waited on it.
	explainBudget = 4
)

// explainURL is the sidecar's explanation route.
//
// A var rather than a const only so that a test can aim it at an httptest
// server. Nothing outside a test writes it, and the sidecar has no discovery
// mechanism of its own, so the address is fixed at the default build.
var explainURL = "http://localhost:8788/explain"

// routing is where requests go, in one value so a request resolves its
// destination with a single atomic load and no lock.
//
// It is a snapshot of the store rather than a second source of truth: the store
// owns the projects, and RefreshRouting copies what it finds. One value holding
// both the active project and the targets keeps the two from disagreeing — the
// alternative was an active-project atomic beside a targets atomic, and a
// request that read them either side of a switch would have dialled one
// client's backend under another client's name.
type routing struct {
	active  string
	targets map[string]targetEntry
	// gen is the store generation this snapshot was read at. getTargetURL
	// compares it against the store's current one, so a snapshot that missed a
	// change is corrected on the next request rather than persisting until
	// somebody remembers to call RefreshRouting. See storage.routingGen.
	gen uint64
}

// targetEntry is where one project's requests go. The SSRF decision that
// produced it is not carried here: it is what authorised the target, not part of
// addressing it, and the store already holds it as the project's
// TargetAllowPrivate. It was duplicated here for a while and read by nothing,
// which is worse than absent — a security field sitting in the request path
// reads like a check happening there.
type targetEntry struct {
	url *url.URL
}

type Proxy struct {
	routing       atomic.Pointer[routing]
	store         *storage.Store
	hub           *events.Hub
	mockCtrl      *mock.MockController
	transport     *http.Transport
	server        *http.Server
	droppedAlerts int64 // counter for dropped breaking alerts

	// explainSlots is the concurrency budget for sidecar calls. A buffered
	// channel rather than a semaphore dependency, because a non-blocking send is
	// exactly the "drop it when full" behaviour wanted here.
	explainSlots chan struct{}
}

func NewProxy(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	return newProxyWithTarget(target, store, hub, mockCtrl, false)
}

// NewProxyAllowPrivate builds a proxy that accepts a private target.
//
// It is the right constructor when the target came from the operator — the
// --target flag, or a test's own httptest server — because an operator naming
// http://localhost:3000 is naming the thing they want sniffed, and that is the
// product's primary use. It is the wrong constructor for a target that arrived
// over the network; use SetProjectTarget for those.
//
// It was called NewProxyForTest and called from main, which is how the SSRF
// blocklist came to be disabled in every shipped binary.
func NewProxyAllowPrivate(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	return newProxyWithTarget(target, store, hub, mockCtrl, true)
}

// NewProxyWithoutTarget builds a proxy for an install whose active project has
// nowhere to dial.
//
// A project with no target is a state the dashboard creates rather than an
// error — CreateProject documents it as legitimate, and RefreshRouting already
// leaves such a project out of the routing snapshot, so every request to it
// answers 502 "no target configured". Startup was the one place that treated
// the state as fatal, and refusing to start took every *other* project down
// with the one that had no backend: a client with no backend of its own could
// stop the operator monitoring the clients that had one.
//
// Nothing is written to the store here. There is no target to write, and the
// store is where a target comes from rather than where one is invented, so the
// active project stays targetless until somebody sets one — which is what makes
// this constructor honest, and what keeps it from quietly promoting the flag's
// target onto a project the operator left empty on purpose.
func NewProxyWithoutTarget(store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	p := newProxy(store, hub, mockCtrl)
	return p, p.RefreshRouting()
}

// newProxyWithTarget records the operator's target as the active project's and
// takes the first routing snapshot from the store.
//
// The target is written into the store rather than kept beside it. A proxy
// holding its own copy of the target would be a second answer to a question the
// store already answers, and the two would diverge the first time a project was
// switched — the request path would still be dialling the target the process
// started with while every view showed the new project's.
func newProxyWithTarget(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController, allowPrivate bool) (*Proxy, error) {
	parsed, err := parseAndValidateTarget(target, allowPrivate)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	p := newProxy(store, hub, mockCtrl)
	if err := p.storeTarget(store.ActiveProject(), parsed, allowPrivate); err != nil {
		return nil, err
	}
	return p, nil
}

// newProxy is the one place a Proxy is assembled. The two constructors above
// differ only in whether SSRF checks are on, and they had already drifted: a
// field added to one and not the other left the test proxy with a nil
// explainSlots, and a send on a nil channel takes the default branch forever, so
// every explanation was silently dropped in exactly the tests meant to catch
// that.
func newProxy(store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) *Proxy {
	return &Proxy{
		store:        store,
		hub:          hub,
		mockCtrl:     mockCtrl,
		transport:    newTransport(),
		explainSlots: make(chan struct{}, explainBudget),
	}
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}

func parseAndValidateTarget(raw string, allowPrivate bool) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("target URL cannot be empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}

	if !allowPrivate && isBlockedHost(parsed.Hostname()) {
		return nil, fmt.Errorf("target host %q is blocked (SSRF protection)", parsed.Hostname())
	}
	return parsed, nil
}

// isBlockedHost reports whether a target host names infrastructure the proxy
// has no business reaching.
//
// Only SetTarget consults this. The constructor does not, because Driftwood's
// own default target is http://localhost:3000 — it is a local dev tool, and a
// rule that blocked private hosts unconditionally would reject its primary use.
// The line that matters is not private-versus-public but who named the target:
// an operator typing a flag, or a request that arrived over the wire.
func isBlockedHost(host string) bool {
	// Hostnames are case-insensitive, and a trailing dot is a legal fully
	// qualified spelling of the same name. A plain string compare let both
	// spellings of "localhost" through.
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return true
	}

	switch host {
	case "localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback":
		return true
	case "metadata.google.internal", "metadata.goog":
		// The GCP metadata service answers to these names, which are not IP
		// literals, so nothing below would catch them.
		return true
	}

	if ip := parseIPLiteral(host); ip != nil {
		return isBlockedIP(ip)
	}

	// A name with no dot resolves against the search domains, so it names
	// something inside the network Driftwood runs in rather than a host on the
	// public internet. A dot is not proof of safety, but its absence is proof
	// of locality.
	return !strings.Contains(host, ".")
}

// isBlockedIP reports whether an address is one Driftwood will not dial. The
// stdlib predicates cover the ranges the hand-rolled octet comparisons did,
// plus IPv6 ULA (fd00::/8) and the unspecified address, both of which were
// reachable before.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// parseIPLiteral reads the spellings a resolver accepts for an address but
// net.ParseIP does not: the single-integer decimal and hexadecimal forms. Both
// 2130706433 and 0x7f000001 mean 127.0.0.1 to getaddrinfo, which is what
// actually dials them, so a check that only understood dotted quads waved them
// straight through.
func parseIPLiteral(host string) net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}

	base, digits := 10, host
	if strings.HasPrefix(host, "0x") || strings.HasPrefix(host, "0X") {
		base, digits = 16, host[2:]
	}
	if digits == "" {
		return nil
	}
	n, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return nil
	}
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// ErrInvalidTarget reports that a target was refused on its merits: a scheme
// that is not http or https, or a host the SSRF rules will not dial.
//
// It is a sentinel because the caller has to tell this apart from a failure to
// *record* a target that was accepted. Both are errors and neither is a 200, but
// they are different answers to the client — 400 for "you asked for something I
// will not do", 500 for "I agreed and could not write it down" — and a handler
// that cannot distinguish them reports the second as the first. That is not
// hypothetical: this feature's first version surfaced an unwritable data
// directory as "invalid target URL", and the existing test for that route is
// what said so.
var ErrInvalidTarget = errors.New("invalid target")

// SetProjectTarget points one project at a new backend.
//
// The target is validated rather than trusted, and allowPrivate is the
// caller's assertion that the request which carried it came from the operator.
// Driftwood has no authentication of its own and runs inside networks worth
// reaching, so a retarget from off-box must not be able to aim it at
// link-local metadata or the LAN behind it.
//
// The validation happens here, before the store is touched, because this is the
// only place that knows where the request came from. The store records the
// outcome; it does not re-derive it.
//
// An error that is ErrInvalidTarget means the target was refused. Any other
// error means it was accepted and could not be saved.
func (p *Proxy) SetProjectTarget(projectID, target string, allowPrivate bool) error {
	if !p.knownProject(projectID) {
		return fmt.Errorf("no project %q", projectID)
	}
	parsed, err := parseAndValidateTarget(target, allowPrivate)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTarget, err)
	}
	return p.storeTarget(projectID, parsed, allowPrivate)
}

// storeTarget records a validated target against a project and republishes the
// routing snapshot.
//
// Kept apart from SetProjectTarget so the constructors can take this path
// without the project-existence check: they run before any project could have
// been made, against a store that has just seeded its first one.
func (p *Proxy) storeTarget(projectID string, parsed *url.URL, allowPrivate bool) error {
	if err := p.store.SetProjectTarget(projectID, parsed.String(), allowPrivate); err != nil {
		return err
	}
	return p.RefreshRouting()
}

func (p *Proxy) knownProject(projectID string) bool {
	return p.store.ProjectExists(projectID)
}

// ValidateTarget reports whether a target would be accepted, without recording it.
//
// It exists so creating a project can check its target before the project is
// made. The alternative was to create first and delete on failure, which leaves
// a window where a project exists holding a target nobody validated — and a
// rollback path that has to be right, in the one place a mistake means a client
// pointing at nothing. It is the same check SetProjectTarget makes: one
// implementation, reached two ways.
func (p *Proxy) ValidateTarget(target string, allowPrivate bool) error {
	if _, err := parseAndValidateTarget(target, allowPrivate); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTarget, err)
	}
	return nil
}

// RefreshRouting rebuilds the proxy's view of where requests go from the store.
//
// Everything that changes a project's target, or which project is active, has
// to end here — the store is the source of truth and this is the copy the
// request path reads. It is one function rather than a setter per field so that
// a new mutation has one obvious place to hook into, and so the snapshot cannot
// be assembled from a store that moved underneath it: a target map read after a
// switch but an active id read before it would dial one client's backend under
// another client's name.
//
// A project whose stored target does not parse is left out of the snapshot
// rather than failing the whole refresh. The document is a file on disk that a
// person can edit, so this is reachable; dropping the one unusable project keeps
// the other clients working, and that project's requests get the same answer as
// a project with no target, which is honest about there being nowhere to go.
func (p *Proxy) RefreshRouting() error {
	active, targets, gen := p.store.Routing()

	next := &routing{active: active, gen: gen, targets: make(map[string]targetEntry, len(targets))}
	for id, decision := range targets {
		if decision.URL == "" {
			continue
		}
		parsed, err := parseAndValidateTarget(decision.URL, decision.AllowPrivate)
		if err != nil {
			continue
		}
		next.targets[id] = targetEntry{url: parsed}
	}
	p.setRouting(next)
	return nil
}

func (p *Proxy) setRouting(r *routing) {
	p.routing.Store(r)
}

// getTargetURL is the request path's routing cost: one atomic load, and a
// second one to check it is current.
//
// It reads the active project and the target from the same value, so a switch
// landing mid-request cannot pair a new project with an old backend.
func (p *Proxy) getTargetURL() *url.URL {
	r := p.loadRouting()
	if r == nil {
		return nil
	}
	entry, ok := r.targets[r.active]
	if !ok {
		return nil
	}
	return entry.url
}

// loadRouting returns a snapshot known to be current, rebuilding it if the store
// has moved on.
//
// The snapshot is a copy the request path reads without a lock, and a copy only
// stays correct if something keeps it in step. Every mutation that changes
// routing does call RefreshRouting — but that is a rule a future mutation has to
// remember, and the cost of forgetting is not a stale dashboard, it is a
// request delivered to the previous client's backend. Comparing generations
// turns that from a thing to remember into a thing that cannot happen: the
// store bumps a counter on every routing change, this checks it, and a snapshot
// that missed one is discarded before it is used.
//
// The extra load is on the request path and costs an atomic read. The rebuild
// happens at most once per change, not once per request: the first request after
// a change refreshes, and the rest find the generation matching.
//
// If rebuilding fails the previous snapshot is returned rather than nothing.
// RefreshRouting does not currently fail, and a stale answer to the previous
// backend is still a better one than refusing to proxy at all — but that is a
// judgement worth revisiting if it ever grows a real failure mode.
func (p *Proxy) loadRouting() *routing {
	r := p.routing.Load()
	if r != nil && r.gen == p.store.RoutingGeneration() {
		return r
	}
	if err := p.RefreshRouting(); err != nil {
		return r
	}
	return p.routing.Load()
}

// GetTargetURLForTest reports where the active project's requests are going.
//
// It replaced a hook that handed tests the atomic itself so they could store an
// unvalidated URL into it. That was a second way into the routing table which
// skipped every check on the way — and a test that reaches a backend by a route
// production does not have is not testing production. Tests retarget through
// SetProjectTarget now, which is the same call the API makes.
func (p *Proxy) GetTargetURLForTest() *url.URL {
	return p.getTargetURL()
}

func (p *Proxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqBodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxAnalyzedBody))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		// The cap bounds what Driftwood records, not what the backend receives.
		// Handing on the capped copy meant a request larger than the cap reached
		// the backend truncated, still under the Content-Length the client had
		// sent — so the backend either read a short body or waited for bytes that
		// were never coming. LimitReader stops at the prefix without consuming
		// past it, so the two readers together are the whole body.
		r.Body = &multiReadCloser{
			Reader: io.MultiReader(bytes.NewReader(reqBodyBytes), r.Body),
			Closer: r.Body,
		}

		reqHeaders := make(map[string]string)
		for k, v := range r.Header {
			reqHeaders[k] = strings.Join(v, ", ")
		}

		// IsMockPath, not HasPrefix(MockPrefix+"/"): the bare "/_driftwood/mock"
		// is a mock request too. It used to fall through to the real backend
		// while the router had already decided it was a mock path.
		if IsMockPath(r.URL.Path) {
			p.serveMockResponse(w, r, start, reqBodyBytes, reqHeaders)
			return
		}

		p.executeProxyCall(w, r, start, reqBodyBytes, reqHeaders)
	}
}

func (p *Proxy) executeProxyCall(w http.ResponseWriter, r *http.Request, start time.Time, reqBodyBytes []byte, reqHeaders map[string]string) {
	target := p.getTargetURL()
	if target == nil {
		http.Error(w, "no target configured", http.StatusBadGateway)
		return
	}

	revProxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
		},
		Transport: p.transport,
	}

	var respBodyBuf bytes.Buffer
	var respStatusCode int = http.StatusOK
	respHeaders := make(map[string]string)

	revProxy.ModifyResponse = func(resp *http.Response) error {
		respStatusCode = resp.StatusCode
		for k, v := range resp.Header {
			respHeaders[k] = strings.Join(v, ", ")
		}

		// Read a bounded prefix to analyze, and pass the rest through untouched.
		//
		// These used to be one set of bytes: the body was capped at 10 MiB and the
		// capped copy was what got served. A response over the cap therefore
		// arrived truncated under the Content-Length the backend had sent, so the
		// client was told to expect bytes that never came. Driftwood is a proxy;
		// silently corrupting the traffic it is watching is a worse outcome than
		// not looking at it, so the cap now bounds the analysis only.
		analyzed, err := io.ReadAll(io.LimitReader(resp.Body, maxAnalyzedBody))
		if err != nil {
			return err
		}
		resp.Body = &multiReadCloser{
			Reader: io.MultiReader(bytes.NewReader(analyzed), resp.Body),
			Closer: resp.Body,
		}

		// A gzipped body is decompressed to be read, not to be served. The client
		// asked for gzip and the response says so, so it gets the compressed
		// stream it was promised: rewriting the body to identity while leaving
		// Content-Encoding: gzip in place handed a client that had sent
		// Accept-Encoding: gzip a body it would then fail to decode.
		analysisBody := analyzed
		if ce := resp.Header.Get("Content-Encoding"); strings.Contains(strings.ToLower(ce), "gzip") {
			decompressed, derr := decompressGzip(analyzed, maxAnalyzedBody)
			if derr != nil && len(decompressed) == 0 {
				log.Printf("[Driftwood] gzip decompress error: %v", derr)
			} else {
				analysisBody = decompressed
				// The recorded body is the decompressed one, so the recorded
				// headers have to say so. respHeaders is a copy of resp.Header,
				// not the header the client is served.
				respHeaders["Content-Encoding"] = "identity"
			}
		}

		respBodyBuf.Write(analysisBody)
		return nil
	}

	revProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Driftwood Proxy Error] %v", err)

		// Check if dev mock mode is enabled
		cfg := p.store.GetConfig()
		if cfg.DevMockMode {
			log.Printf("[Driftwood] DevMockMode enabled, serving mock response")
			p.serveMockResponse(w, r, time.Now(), nil, nil)
			return
		}

		// Return 502 instead of silently falling back to mock
		http.Error(w, "Bad Gateway: target unreachable", http.StatusBadGateway)
	}

	recWriter := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		header:         w.Header(),
	}

	revProxy.ServeHTTP(recWriter, r)

	// Process captured transaction (always process for sniffing, including 204/empty)
	duration := time.Since(start).Milliseconds()
	respStr := respBodyBuf.String()
	isJSON := isJSONContent(respHeaders, respStr)

	// Resolved once, here, where the request begins. Everything downstream of
	// this takes the project as a parameter, so a single request is recorded
	// entirely under one project even if the active project changes while it is
	// in flight — which it can, because switching is a dashboard action and
	// requests take as long as the backend takes.
	p.processAndStoreTraffic(
		p.store.ActiveProject(),
		r.Method,
		r.URL.Path,
		r.URL.String(),
		respStatusCode,
		duration,
		reqHeaders,
		respHeaders,
		string(reqBodyBytes),
		respStr,
		isJSON,
	)
}

// maxAnalyzedBody caps how much of a message Driftwood reads in order to diff
// it. It bounds the analysis, not the traffic: everything past it is forwarded
// to its destination untouched.
const maxAnalyzedBody = 10 << 20 // 10 MiB

// multiReadCloser reads a prefix followed by the remainder of an original body,
// and closes that original when it is closed.
//
// It exists because the two obvious spellings are both wrong here. Wrapping a
// MultiReader in io.NopCloser drops the original's Close, so the connection it
// came from is never released for reuse; closing the original up front, which is
// what this code did while it was also replacing the body, is what made the
// remainder unreadable. Handing the original on as the Closer keeps both.
type multiReadCloser struct {
	io.Reader
	io.Closer
}

// decompressGzip expands a gzip body, giving up after limit bytes.
//
// The limit is what makes this safe to point at a response Driftwood did not
// produce. The caller caps the compressed bytes it reads, and that cap bounds
// nothing: ratios of 1000:1 are ordinary, so a few hundred KB of well-chosen
// input expands to gigabytes, and an unbounded ReadAll would allocate every byte
// of it before anyone could object. Reading at most limit bytes from the
// decompressor caps the allocation at the limit, whatever the payload claims.
//
// Bytes that did decompress are returned even alongside an error. The input is a
// prefix of a larger stream whenever the compressed side hit the cap, and a
// truncated gzip member ends in an error — so discarding the output there would
// mean the largest responses are the only ones never diffed.
func decompressGzip(data []byte, limit int64) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(io.LimitReader(gr, limit))
}

func (p *Proxy) serveMockResponse(w http.ResponseWriter, r *http.Request, start time.Time, reqBodyBytes []byte, reqHeaders map[string]string) {
	rec := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		header:         w.Header(),
		body:           &bytes.Buffer{},
	}

	p.mockCtrl.ServeHTTP(rec, r)

	duration := time.Since(start).Milliseconds()
	respStr := rec.body.String()
	respHeaders := make(map[string]string)
	for k, v := range rec.Header() {
		respHeaders[k] = strings.Join(v, ", ")
	}

	// Resolved once, here, where the request begins. Everything downstream of
	// this takes the project as a parameter, so a single request is recorded
	// entirely under one project even if the active project changes while it is
	// in flight — which it can, because switching is a dashboard action and
	// requests take as long as the backend takes.
	p.processAndStoreTraffic(
		p.store.ActiveProject(),
		r.Method,
		r.URL.Path,
		r.URL.String(),
		rec.statusCode,
		duration,
		reqHeaders,
		respHeaders,
		string(reqBodyBytes),
		respStr,
		true,
	)
}

// assessFailedResponse reports a non-2xx response as the status change it is,
// rather than diffing the error body against a contract it never claimed to
// satisfy.
//
// The severity split is the point. A 5xx is the service failing to honour the
// contract, and is published as an alert. A 4xx is the contract being enforced
// against the caller, so it is filed as a warning instead: it still reaches the
// traffic list and the alert log, where an endpoint that stopped answering 200
// belongs, but it never broadcasts and is never described as a break.
func (p *Proxy) assessFailedResponse(projectID, method, path string, statusCode int) (string, *types.ContractDiff) {
	// An error body is not a shape to promise. Auto-saving one here would make
	// the failure itself the baseline, and every later success look like drift.
	if _, exists := p.store.GetBaseline(projectID, method, path); !exists {
		return "NO_BASELINE", nil
	}

	severity := types.SeverityWarning
	status := "WARNING"
	if statusCode >= 500 {
		severity = types.SeverityBreaking
		status = "BREAKING"
	}

	return status, &types.ContractDiff{
		HasBreakingChanges: status == "BREAKING",
		HasWarnings:        status == "WARNING",
		Deltas: []types.DiffDelta{
			{
				JSONPath: "$",
				Kind:     types.KindStatusCodeChange,
				Severity: severity,
				Message:  fmt.Sprintf("endpoint answered with HTTP %d; the contract describes a successful response", statusCode),
				Expected: "2xx",
				Actual:   fmt.Sprintf("%d", statusCode),
			},
		},
	}
}

func (p *Proxy) processAndStoreTraffic(
	projectID string,
	method, path, rawURL string,
	statusCode int,
	durationMs int64,
	reqHeaders, respHeaders map[string]string,
	reqBody, respBody string,
	isJSON bool,
) {
	contractStatus := "NO_BASELINE"
	var contractDiff *types.ContractDiff

	sanitizedReqHeaders := sanitizeHeaders(reqHeaders)
	sanitizedRespHeaders := sanitizeHeaders(respHeaders)

	// A failed response is not evidence about the contract's shape, so it is
	// assessed as the failure it is and never diffed against a success schema.
	// Otherwise a single transient 500 on a healthy API reports every field of
	// the real response as REMOVED_FIELD and raises a breaking alert naming all
	// of them — noise that buries the one fact worth knowing.
	if statusCode >= 400 {
		contractStatus, contractDiff = p.assessFailedResponse(projectID, method, path, statusCode)
	} else if isJSON && strings.TrimSpace(respBody) != "" {
		baseline, exists := p.store.GetBaseline(projectID, method, path)
		if !exists {
			cfg := p.store.GetConfig()
			if cfg.AutoSaveBaseline {
				// Recorded as auto, which is what makes the version read as
				// provisional: this response was never checked against anything,
				// so it is evidence of what the API returns, not of what it
				// promised. Confirming it in the dashboard is what turns it into
				// a contract.
				_, _ = p.store.SaveBaselineFrom(projectID, method, path, respBody, types.BaselineSourceAuto)
				contractStatus = "BASELINE_SET"
			}
		} else {
			// Compared against the stored schema, not a re-inference of the
			// stored sample. Re-inferring discards everything the baseline knows
			// beyond the shape of one response — an imported spec's `required`
			// list most of all — and silently replaces it with "every key in the
			// sample is required". Falls back to the sample when a baseline
			// predates having a schema stored.
			var d *types.ContractDiff
			var err error
			if baseline.Schema != nil {
				d, err = diff.CompareJSONWithSchema(baseline.Schema, respBody)
			} else {
				d, err = diff.CompareJSON(baseline.SamplePayload, respBody)
			}
			if err == nil {
				contractDiff = d
				if d.HasBreakingChanges {
					contractStatus = "BREAKING"
				} else if d.HasWarnings {
					contractStatus = "WARNING"
				} else {
					contractStatus = "MATCH"
				}
			}
		}
	} else if statusCode == http.StatusNoContent {
		// For 204 No Content, still check if we have a baseline to detect contract violation
		_, exists := p.store.GetBaseline(projectID, method, path)
		if exists {
			contractStatus = "BREAKING" // expected body but got none
			contractDiff = &types.ContractDiff{
				HasBreakingChanges: true,
				Deltas: []types.DiffDelta{
					{
						JSONPath: "$",
						Kind:     types.KindTypeMismatch,
						Severity: types.SeverityBreaking,
						Message:  "expected response body but received 204 No Content",
						Expected: "JSON body",
						Actual:   "<empty>",
					},
				},
			}
		}
	}

	sanitizedReqBody := capture.SanitizeBody(reqBody)
	sanitizedRespBody := capture.SanitizeBody(respBody)

	traffic := types.CapturedTraffic{
		ID:              fmt.Sprintf("tr_%d", time.Now().UnixNano()),
		Timestamp:       time.Now(),
		Method:          method,
		Path:            path,
		URL:             rawURL,
		StatusCode:      statusCode,
		DurationMs:      durationMs,
		RequestHeaders:  sanitizedReqHeaders,
		ResponseHeaders: sanitizedRespHeaders,
		RequestBody:     sanitizedReqBody,
		ResponseBody:    sanitizedRespBody,
		IsJSON:          isJSON,
		ContractStatus:  contractStatus,
		Diff:            contractDiff,
	}

	capture.SanitizeTraffic(&traffic)

	p.store.AddTraffic(projectID, traffic)
	p.hub.Publish(projectID, "traffic", traffic)

	if contractDiff != nil && contractDiff.HasBreakingChanges {
		alertData := map[string]interface{}{
			"traffic_id":      traffic.ID,
			"endpoint":        fmt.Sprintf("%s %s", method, path),
			"contract_status": contractStatus,
			"diff":            contractDiff,
		}

		/* The alert goes out first, and the explanation follows it if it arrives.

		   It cannot go out with the alert: the sidecar call takes up to 8s, and
		   waiting on an optional paragraph would delay every breaking-change
		   notification by a round trip to a service that is allowed to be absent.

		   What stood here tried to have both. The goroutine wrote
		   alertData["ai_explanation"] while Publish was marshalling that same
		   map, so the write landed after the bytes had already gone and the
		   published alert never carried an explanation — and the read and the
		   write were unsynchronized, which the race detector flags and which can
		   crash the process outright.

		   The explanation now travels the other way: the goroutine files it
		   against the stored alert through UpdateAlertAIExplanation, the seam
		   that method was written for, and then announces that it landed. */
		p.hub.Publish(projectID, "alert", alertData)

		/* The `go` is the whole of "the explanation follows it".

		   Without it this call is synchronous, and every property the block above
		   claims stops being true. The alert still goes out — it is published on
		   the line above — but the handler then sits on a sidecar round trip before
		   it can return, and the caller pays for that on the one path where this
		   product is supposed to be invisible.

		   That the response was already written by the time we get here does not
		   save it, which is the part worth knowing: the proxy writes through a
		   responseRecorder to the client, but net/http holds a small body in its
		   write buffer and flushes it when the handler returns, not when it is
		   written. So a response under the buffer size reaches the client only
		   after this call finishes. Measured at the full 8s client timeout in
		   TestBreakingChangeDoesNotWaitOnTheSidecar, with the sidecar held open. */
		go p.explainAsync(projectID, traffic.ID, method, path, contractDiff, respBody)
	}
}

// explainAsync asks the AI sidecar to explain a breaking change and files the
// answer against the alert it belongs to.
//
// Nothing here is a closure over the handler's locals, and that is deliberate:
// the handler returns immediately, so anything it still owns is both a lifetime
// hazard and, in the case of a map it shares with a concurrent Marshal, a data
// race. Every value this needs arrives as a parameter.
func (p *Proxy) explainAsync(projectID, trafficID, method, path string, contractDiff *types.ContractDiff, respBody string) {
	select {
	case p.explainSlots <- struct{}{}:
	default:
		// Saturated. The alert is already delivered, so the optional half is
		// the half that goes.
		return
	}
	defer func() { <-p.explainSlots }()

	deltas, err := json.Marshal(contractDiff.Deltas)
	if err != nil {
		return
	}

	/* Both samples are redacted here, on the way out.

	   They leave the process for a third-party API, and a response body is the
	   likeliest place in this product for a token or an email address to be
	   sitting. Baselines are stored raw, so sanitizing the stored copy would be
	   a migration over data already on disk; redacting at the boundary means
	   the copy that leaves is clean no matter when the baseline was written. */
	var baselineSample string
	if baseline, ok := p.store.GetBaseline(projectID, method, path); ok && baseline != nil {
		baselineSample = capture.SanitizeBody(baseline.SamplePayload)
	}

	bodyBytes, err := json.Marshal(map[string]interface{}{
		"endpoint":        fmt.Sprintf("%s %s", method, path),
		"deltas":          json.RawMessage(deltas),
		"baseline_sample": baselineSample,
		"current_sample":  capture.SanitizeBody(respBody),
	})
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", explainURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Deliberately not NewRequestWithContext: the context in scope belongs to
	// the proxied request and is cancelled the moment its handler returns, which
	// is before this goroutine has finished. The client timeout is the bound it
	// actually needs, and the sidecar is local.
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var explanation map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&explanation); err != nil || len(explanation) == 0 {
		return
	}

	if err := p.store.UpdateAlertAIExplanation(trafficID, explanation); err != nil {
		// The alert aged out of the ring buffer while the sidecar was thinking.
		// There is nothing left to attach this to and nothing wrong.
		return
	}

	/* A dashboard that was already open received the alert over SSE without the
	   explanation, so the explanation needs its own announcement.

	   A distinct event type rather than a second "alert": re-publishing the
	   alert would fire the browser's breaking-change toast a second time for a
	   change it has already announced. */
	p.hub.Publish(projectID, "alert_explained", map[string]interface{}{"traffic_id": trafficID})
}

func sanitizeHeaders(headers map[string]string) map[string]string {
	sensitive := map[string]bool{
		"authorization":       true,
		"cookie":              true,
		"set-cookie":          true,
		"x-api-key":           true,
		"x-auth-token":        true,
		"proxy-authorization": true,
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		if sensitive[strings.ToLower(k)] {
			result[k] = "[REDACTED]"
		} else {
			result[k] = v
		}
	}
	return result
}

func isJSONContent(headers map[string]string, body string) bool {
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			if strings.Contains(strings.ToLower(v), "application/json") {
				return true
			}
		}
	}
	trimmed := strings.TrimSpace(body)
	return (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	header     http.Header
	body       *bytes.Buffer
}

func (r *responseRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.body != nil {
		r.body.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

// Shutdown gracefully stops the proxy
func (p *Proxy) Shutdown(ctx context.Context) error {
	if p.server != nil {
		return p.server.Shutdown(ctx)
	}
	return nil
}

// DroppedAlerts returns the count of dropped alerts
func (p *Proxy) DroppedAlerts() int64 {
	return atomic.LoadInt64(&p.droppedAlerts)
}
