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

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// CORS headers for local DevTools or API integration
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Driftwood-Target")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Direct UI or Driftwood Control API call
		if path == "/" || strings.HasPrefix(path, "/_driftwood") {
			switch {
			case path == "/" || path == "/_driftwood" || path == "/_driftwood/":
				web.ServeIndex(w, r)
			case path == "/_driftwood/events":
				s.hub.SSEHandler(w, r)
			case path == "/_driftwood/api/traffic":
				s.handleTraffic(w, r)
			case path == "/_driftwood/api/traffic/clear":
				s.handleClearTraffic(w, r)
			case path == "/_driftwood/api/baselines":
				s.handleBaselines(w, r)
			case path == "/_driftwood/api/baselines/delete":
				s.handleDeleteBaseline(w, r)
			case path == "/_driftwood/api/config":
				s.handleConfig(w, r)
			case path == "/_driftwood/api/alerts":
				s.handleAlerts(w, r)
			case path == "/_driftwood/api/export/typescript":
				s.handleExportTypeScript(w, r)
			case path == "/_driftwood/api/mock/mode":
				s.handleMockMode(w, r)
			case strings.HasPrefix(path, "/_driftwood/mock"):
				proxyHandler(w, r)
			default:
				web.ServeIndex(w, r)
			}
			return
		}

		// Forward all other requests through the sniffing proxy
		proxyHandler(w, r)
	}
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
