package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
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

type Proxy struct {
	targetURL       atomic.Pointer[url.URL]
	store           *storage.Store
	hub             *events.Hub
	mockCtrl        *mock.MockController
	transport       *http.Transport
	server          *http.Server
	allowPrivate    bool // for testing/dev - bypass SSRF for private IPs
	droppedAlerts   int64 // counter for dropped breaking alerts
}

func NewProxy(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	parsed, err := parseAndValidateTarget(target, false)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	p := &Proxy{
		store:        store,
		hub:          hub,
		mockCtrl:     mockCtrl,
		transport:    newTransport(),
		allowPrivate: false,
	}
	p.targetURL.Store(parsed)
	return p, nil
}

// NewProxyForTest creates a proxy with SSRF checks disabled for testing
func NewProxyForTest(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	parsed, err := parseAndValidateTarget(target, true)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	p := &Proxy{
		store:        store,
		hub:          hub,
		mockCtrl:     mockCtrl,
		transport:    newTransport(),
		allowPrivate: true,
	}
	p.targetURL.Store(parsed)
	return p, nil
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
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

func isBlockedHost(host string) bool {
	if host == "localhost" || host == "localhost.localdomain" || host == "ip6-localhost" {
		return true
	}
	if host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "127.") {
		return true
	}
	if host == "169.254.169.254" || strings.HasPrefix(host, "169.254.") {
		return true
	}
	if strings.HasPrefix(host, "fe80::") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 10 ||
				(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
				(ip4[0] == 192 && ip4[1] == 168) {
				return true
			}
		}
	}
	return false
}

func (p *Proxy) SetTarget(target string) error {
	parsed, err := parseAndValidateTarget(target, p.allowPrivate)
	if err != nil {
		return err
	}
	p.targetURL.Store(parsed)
	return nil
}

func (p *Proxy) getTargetURL() *url.URL {
	return p.targetURL.Load()
}

func (p *Proxy) GetTargetURLForTest() *atomic.Pointer[url.URL] {
	return &p.targetURL
}

func (p *Proxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqBodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))

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

		// Read body with limit
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		// Decompress gzip if present (Content-Encoding: gzip)
		if ce := resp.Header.Get("Content-Encoding"); strings.Contains(strings.ToLower(ce), "gzip") {
			decompressed, err := decompressGzip(bodyBytes)
			if err != nil {
				log.Printf("[Driftwood] gzip decompress error: %v", err)
			} else {
				bodyBytes = decompressed
				// Remove Content-Encoding since we decompressed
				respHeaders["Content-Encoding"] = "identity"
			}
		}

		respBodyBuf.Write(bodyBytes)
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
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

	p.processAndStoreTraffic(
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

func decompressGzip(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
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

	p.processAndStoreTraffic(
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

func (p *Proxy) processAndStoreTraffic(
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

	// For 204/empty responses, still process if we have a baseline to check
	if isJSON && strings.TrimSpace(respBody) != "" {
		baseline, exists := p.store.GetBaseline(method, path)
		if !exists {
			cfg := p.store.GetConfig()
			if cfg.AutoSaveBaseline {
				// Recorded as auto, which is what makes the version read as
				// provisional: this response was never checked against anything,
				// so it is evidence of what the API returns, not of what it
				// promised. Confirming it in the dashboard is what turns it into
				// a contract.
				_, _ = p.store.SaveBaselineFrom(method, path, respBody, types.BaselineSourceAuto)
				contractStatus = "BASELINE_SET"
			}
		} else {
			d, err := diff.CompareJSON(baseline.SamplePayload, respBody)
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
		_, exists := p.store.GetBaseline(method, path)
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

	p.store.AddTraffic(traffic)
	p.hub.Publish("traffic", traffic)

	if contractDiff != nil && contractDiff.HasBreakingChanges {
		alertData := map[string]interface{}{
			"traffic_id":      traffic.ID,
			"endpoint":        fmt.Sprintf("%s %s", method, path),
			"contract_status": contractStatus,
			"diff":            contractDiff,
		}

		// Call AI explainer sidecar for breaking changes (async, non-blocking)
		go func() {
			// Get baseline if exists
			var baselineSample string
			baseline, hasBaseline := p.store.GetBaseline(method, path)
			if hasBaseline && baseline != nil {
				baselineSample = baseline.SamplePayload
			}

			url := "http://localhost:8788/explain"
			deltas, _ := json.Marshal(contractDiff.Deltas)
			body := map[string]interface{}{
				"endpoint":       fmt.Sprintf("%s %s", method, path),
				"deltas":         json.RawMessage(deltas),
				"baseline_sample": baselineSample,
				"current_sample": respBody,
			}
			bodyBytes, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 8 * time.Second}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				var aiResp map[string]interface{}
				if json.NewDecoder(resp.Body).Decode(&aiResp) == nil {
					alertData["ai_explanation"] = aiResp
				}
				resp.Body.Close()
			} else if resp != nil {
				resp.Body.Close()
			}
		}()

		p.hub.Publish("alert", alertData)
	}
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