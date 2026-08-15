package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
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

type Proxy struct {
	targetURL    atomic.Pointer[url.URL]
	store        *storage.Store
	hub          *events.Hub
	mockCtrl     *mock.MockController
	transport    *http.Transport
	server       *http.Server
	allowPrivate bool // for testing/dev - bypass SSRF for private IPs
}

func NewProxy(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	parsed, err := parseAndValidateTarget(target, false)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	p := &Proxy{
		store:     store,
		hub:       hub,
		mockCtrl:  mockCtrl,
		transport: newTransport(),
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

	// Block SSRF: reject localhost, private IPs, metadata endpoints (unless allowed)
	if !allowPrivate && isBlockedHost(parsed.Hostname()) {
		return nil, fmt.Errorf("target host %q is blocked (SSRF protection)", parsed.Hostname())
	}
	return parsed, nil
}

func isBlockedHost(host string) bool {
	// localhost variants
	if host == "localhost" || host == "localhost.localdomain" || host == "ip6-localhost" {
		return true
	}
	// loopback
	if host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "127.") {
		return true
	}
	// link-local
	if host == "169.254.169.254" || strings.HasPrefix(host, "169.254.") {
		return true
	}
	// IPv6 link-local
	if strings.HasPrefix(host, "fe80::") {
		return true
	}
	// RFC 1918 private ranges
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
		// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
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

// GetTargetURLForTest returns the atomic pointer for test manipulation
func (p *Proxy) GetTargetURLForTest() *atomic.Pointer[url.URL] {
	return &p.targetURL
}

func (p *Proxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Read request body (streaming with limit)
		reqBodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))

		reqHeaders := make(map[string]string)
		for k, v := range r.Header {
			reqHeaders[k] = strings.Join(v, ", ")
		}

		// Handle built-in mock endpoint
		if strings.HasPrefix(r.URL.Path, "/_driftwood/mock/") {
			p.serveMockResponse(w, r, start, reqBodyBytes, reqHeaders)
			return
		}

		// Proxy request to target server
		p.executeProxyCall(w, r, start, reqBodyBytes, reqHeaders)
	}
}

func (p *Proxy) executeProxyCall(w http.ResponseWriter, r *http.Request, start time.Time, reqBodyBytes []byte, reqHeaders map[string]string) {
	target := p.getTargetURL()
	if target == nil {
		http.Error(w, "no target configured", http.StatusBadGateway)
		return
	}

	// Create single host reverse proxy (reuse)
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

		// Stream response body with limit (don't buffer whole body if large)
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		respBodyBuf.Write(bodyBytes)
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		return nil
	}

	revProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Driftwood Proxy Error] %v", err)

		// Return 502 instead of silently falling back to mock
		http.Error(w, "Bad Gateway: target unreachable", http.StatusBadGateway)
	}

	// Capture response writer
	recWriter := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		header:         w.Header(),
	}

	revProxy.ServeHTTP(recWriter, r)

	// Process captured transaction if handled by revProxy
	// Also capture 204/empty responses (key on completion, not body length)
	if true { // always process for sniffing
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

	// Sanitize headers BEFORE any processing/storage
	sanitizedReqHeaders := sanitizeHeaders(reqHeaders)
	sanitizedRespHeaders := sanitizeHeaders(respHeaders)

	if isJSON && strings.TrimSpace(respBody) != "" {
		baseline, exists := p.store.GetBaseline(method, path)
		if !exists {
			cfg := p.store.GetConfig()
			if cfg.AutoSaveBaseline {
				_, _ = p.store.SaveBaseline(method, path, respBody)
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
	}

	// Apply sanitization to request/response bodies (PII in bodies)
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

	// Additional sanitization pass on the full traffic object
	capture.SanitizeTraffic(&traffic)

	p.store.AddTraffic(traffic)
	p.hub.Publish("traffic", traffic)

	if contractDiff != nil && contractDiff.HasBreakingChanges {
		p.hub.Publish("alert", map[string]interface{}{
			"traffic_id":      traffic.ID,
			"endpoint":        fmt.Sprintf("%s %s", method, path),
			"contract_status": contractStatus,
			"diff":            contractDiff,
		})
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