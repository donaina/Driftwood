package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/donaina/driftwood/internal/diff"
	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

type Proxy struct {
	targetURL *url.URL
	store     *storage.Store
	hub       *events.Hub
	mockCtrl  *mock.MockController
}

func NewProxy(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController) (*Proxy, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	return &Proxy{
		targetURL: parsed,
		store:     store,
		hub:       hub,
		mockCtrl:  mockCtrl,
	}, nil
}

func (p *Proxy) SetTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil {
		return err
	}
	p.targetURL = parsed
	return nil
}

func (p *Proxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Read request body
		reqBodyBytes, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))

		reqHeaders := make(map[string]string)
		for k, v := range r.Header {
			reqHeaders[k] = strings.Join(v, ", ")
		}

		// Handle built-in mock endpoint target fallback if specified or target offline
		if strings.HasPrefix(r.URL.Path, "/_driftwood/mock/") {
			p.serveMockResponse(w, r, start, reqBodyBytes, reqHeaders)
			return
		}

		// Proxy request to target server
		p.executeProxyCall(w, r, start, reqBodyBytes, reqHeaders)
	}
}

func (p *Proxy) executeProxyCall(w http.ResponseWriter, r *http.Request, start time.Time, reqBodyBytes []byte, reqHeaders map[string]string) {
	// Create single host reverse proxy
	revProxy := httputil.NewSingleHostReverseProxy(p.targetURL)

	// Custom response interceptor
	var respBodyBuf bytes.Buffer
	var respStatusCode int = http.StatusOK
	respHeaders := make(map[string]string)

	revProxy.ModifyResponse = func(resp *http.Response) error {
		respStatusCode = resp.StatusCode
		for k, v := range resp.Header {
			respHeaders[k] = strings.Join(v, ", ")
		}

		// Intercept and buffer body
		bodyBytes, err := io.ReadAll(resp.Body)
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

		// Fallback to internal Mock backend if configured target server is unreachable
		log.Printf("[Driftwood Proxy] Target unreachable (%s). Serving Mock Backend response.", p.targetURL.String())
		p.serveMockResponse(w, r, start, reqBodyBytes, reqHeaders)
	}

	// Capture response writer
	recWriter := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		header:         w.Header(),
	}

	revProxy.ServeHTTP(recWriter, r)

	// Process captured transaction if handled by revProxy
	if respBodyBuf.Len() > 0 {
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

	if isJSON && strings.TrimSpace(respBody) != "" {
		baseline, exists := p.store.GetBaseline(method, path)
		if !exists {
			// Auto save initial baseline contract if auto-save is enabled
			cfg := p.store.GetConfig()
			if cfg.AutoSaveBaseline {
				_, _ = p.store.SaveBaseline(method, path, respBody)
				contractStatus = "BASELINE_SET"
			}
		} else {
			// Diff current payload against baseline contract
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

	traffic := types.CapturedTraffic{
		ID:              fmt.Sprintf("tr_%d", time.Now().UnixNano()),
		Timestamp:       time.Now(),
		Method:          method,
		Path:            path,
		URL:             rawURL,
		StatusCode:      statusCode,
		DurationMs:      durationMs,
		RequestHeaders:  reqHeaders,
		ResponseHeaders: respHeaders,
		RequestBody:     reqBody,
		ResponseBody:    respBody,
		IsJSON:          isJSON,
		ContractStatus:  contractStatus,
		Diff:            contractDiff,
	}

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
