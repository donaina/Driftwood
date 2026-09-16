package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/donaina/driftwood/internal/contract"
	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
	"github.com/donaina/driftwood/web"
)

type Server struct {
	store    *storage.Store
	hub      *events.Hub
	proxy    *proxy.Proxy
	mockCtrl *mock.MockController
}

func NewServer(store *storage.Store, hub *events.Hub, prx *proxy.Proxy, mockCtrl *mock.MockController) *Server {
	return &Server{
		store:    store,
		hub:      hub,
		proxy:    prx,
		mockCtrl: mockCtrl,
	}
}

func (s *Server) Router() http.HandlerFunc {
	proxyHandler := s.proxy.Handler()

	// The dashboard is served with the reserved namespace stripped, because
	// web/index.html references its assets "./"-relative and web.ServeIndex looks
	// them up on disk by URL path. Stripping turns "/_driftwood/assets/x.css" into
	// "/assets/x.css", which resolves under frontend-react/dist. Without it the
	// lookup would be for "_driftwood/assets/x.css" and every asset would 404.
	dashboard := http.StripPrefix(proxy.ControlPrefix, http.HandlerFunc(web.ServeIndex))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// THE PRODUCT. Anything outside the reserved namespace belongs to the
		// target API: it is proxied, and therefore sniffed, diffed, baselined and
		// recorded. This decision used to be gated on a hardcoded "/api/" prefix,
		// so every endpoint outside it — /v1/*, /graphql, /orders — was answered
		// by the dashboard with a 200 and never seen by the sniffer at all. The
		// engine was real; it was simply never handed the traffic.
		if !proxy.IsControlPath(path) {
			proxyHandler(w, r)
			return
		}

		// CORS belongs to the control plane only. A proxied response must carry
		// the backend's own headers untouched: this ResponseWriter and
		// httputil.ReverseProxy share one http.Header map, and ReverseProxy copies
		// backend headers with Add — so setting Access-Control-Allow-Origin here
		// and letting the backend's copy land on top emitted the header twice,
		// which browsers reject outright.
		setControlCORS(w, r)

		// Preflight is answered here for the control API, and only here. This
		// check used to sit above the routing, which meant a browser preflight
		// against a proxied path got Driftwood's answer and the backend never saw
		// it — a silent CORS regression that curl cannot reveal.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// "./"-relative asset URLs resolve against the document's directory, so
		// the bare namespace has no base and would 404 its own stylesheet.
		if path == proxy.ControlPrefix {
			http.Redirect(w, r, proxy.ControlPrefix+"/", http.StatusMovedPermanently)
			return
		}

		// Mock endpoints stand in for the target API, so they behave like it:
		// proxied and recorded, not answered by the control plane.
		if proxy.IsMockPath(path) {
			proxyHandler(w, r)
			return
		}

		switch path {
		case proxy.ControlPrefix + "/events":
			s.hub.SSEHandler(w, r)
		case proxy.ControlPrefix + "/api/traffic":
			s.handleTraffic(w, r)
		case proxy.ControlPrefix + "/api/traffic/clear":
			s.handleClearTraffic(w, r)
		case proxy.ControlPrefix + "/api/baselines":
			s.handleBaselines(w, r)
		case proxy.ControlPrefix + "/api/baselines/delete":
			s.handleDeleteBaseline(w, r)
		case proxy.ControlPrefix + "/api/config":
			s.handleConfig(w, r)
		case proxy.ControlPrefix + "/api/alerts":
			s.handleAlerts(w, r)
		case proxy.ControlPrefix + "/api/histories":
			s.handleHistories(w, r)
		case proxy.ControlPrefix + "/api/export/typescript":
			s.handleExportTypeScript(w, r)
		case proxy.ControlPrefix + "/api/mock/mode":
			s.handleMockMode(w, r)
		default:
			// An unmatched control-API path is a 404 in JSON, not the dashboard.
			// Returning HTML to a client that mistyped an endpoint hides the
			// error behind a 200 and a page that looks fine.
			if strings.HasPrefix(path, proxy.ControlPrefix+"/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"unknown control endpoint"}`))
				return
			}
			dashboard.ServeHTTP(w, r)
		}
	}
}

// setControlCORS writes the control plane's CORS headers. Only explicitly
// allowed local origins are echoed, and the backend's headers are never mixed in
// — see the comment in Router for why that duplication broke the response.
func setControlCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if isAllowedOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		// The response depends on the request's Origin, so caches must key on it
		// rather than serving one origin's headers to another.
		w.Header().Add("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Driftwood-Target")
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	traffics := s.store.GetTraffics(100)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(traffics)
}

func (s *Server) handleClearTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.store.ClearTraffic()
	s.hub.Publish("traffic_cleared", nil)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleBaselines(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Method  string `json:"method"`
			Path    string `json:"path"`
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cb, err := s.store.SaveBaseline(req.Method, req.Path, req.Payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.hub.Publish("baseline_updated", cb)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cb)
		return
	}

	baselines := s.store.GetAllBaselines()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(baselines)
}

func (s *Server) handleDeleteBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.DeleteBaseline(req.Method, req.Path)
	s.hub.Publish("baseline_deleted", req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var cfg types.ProxyConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.store.UpdateConfig(cfg)
		_ = s.proxy.SetTarget(cfg.TargetURL)
		s.hub.Publish("config_updated", cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.GetConfig())
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.store.GetAlerts(50)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(alerts)
}

func (s *Server) handleHistories(w http.ResponseWriter, r *http.Request) {
	histories := s.store.GetAllHistories()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(histories)
}

func (s *Server) handleExportTypeScript(w http.ResponseWriter, r *http.Request) {
	baselines := s.store.GetAllBaselines()
	var sb strings.Builder

	sb.WriteString("// ⚡ Generated by Driftwood TypeScript Exporter\n")
	sb.WriteString("// Auto-generated TypeScript definitions for API contracts\n\n")

	if len(baselines) == 0 {
		sb.WriteString("// No baseline contracts locked yet.\n")
	} else {
		for _, b := range baselines {
			name := strings.Title(strings.ToLower(b.Method)) + cleanInterfaceName(b.Path) + "Response"
			ts := contract.GenerateTypeScriptInterfaces(name, b.Schema)
			sb.WriteString(ts)
			sb.WriteString("\n")
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"driftwood-contracts.d.ts\"")
	_, _ = w.Write([]byte(sb.String()))
}

func cleanInterfaceName(path string) string {
	parts := strings.Split(path, "/")
	var res string
	for _, p := range parts {
		if p == "" || p == "_driftwood" || p == "mock" || p == "api" {
			continue
		}
		p = strings.ReplaceAll(p, ":", "")
		p = strings.ReplaceAll(p, "-", "_")
		if len(p) > 0 {
			res += strings.Title(p)
		}
	}
	if res == "" {
		res = "Endpoint"
	}
	return res
}

func (s *Server) handleMockMode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mockCtrl.SetMode(mock.MockMode(req.Mode))
		s.hub.Publish("mock_mode_changed", map[string]string{"mode": req.Mode})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"mode": s.mockCtrl.GetMode(),
	})
}

// isAllowedOrigin returns true if the origin is localhost (security: no wildcard CORS)
func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	allowed := []string{
		"http://localhost:8787",
		"http://127.0.0.1:8787",
		"http://[::1]:8787",
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
}
