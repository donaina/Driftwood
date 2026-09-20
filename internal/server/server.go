package server

import (
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/donaina/driftwood/internal/contract"
	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/site"
	"github.com/donaina/driftwood/web"
)

type Server struct {
	store    *storage.Store
	hub      *events.Hub
	proxy    *proxy.Proxy
	mockCtrl *mock.MockController

	// site serves the marketing site when the operator asked for it, and is nil
	// otherwise. Nil is the default and the common case: the site claims "/", and
	// "/" is otherwise the proxied target, so turning it on takes a decision.
	//
	// Held as an http.Handler rather than as a bool so a test can install a stub
	// without reaching into the embed, and so this package does not have to know
	// how the site is stored to hand it a request.
	site http.Handler
}

func NewServer(store *storage.Store, hub *events.Hub, prx *proxy.Proxy, mockCtrl *mock.MockController, siteHandler http.Handler) *Server {
	return &Server{
		store:    store,
		hub:      hub,
		proxy:    prx,
		mockCtrl: mockCtrl,
		site:     siteHandler,
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
			// The marketing site, when the operator asked for it.
			//
			// Inside this branch rather than beside it, deliberately: the site is
			// then structurally incapable of shadowing /_driftwood/*, rather than
			// merely not doing so today. A path the site claims but has no file
			// for is a 404 from site.ServeSite — it must never fall through to the
			// proxy, and it must never answer with index.html, which is how every
			// unknown asset returning 200 and the page itself has shipped here
			// before.
			if s.site != nil && site.Claims(path) {
				s.site.ServeHTTP(w, r)
				return
			}
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
		case proxy.ControlPrefix + "/api/baselines/lock":
			s.handleLockBaseline(w, r)
		case proxy.ControlPrefix + "/api/baselines/confirm":
			s.handleConfirmBaseline(w, r)
		case proxy.ControlPrefix + "/api/config":
			s.handleConfig(w, r)
		case proxy.ControlPrefix + "/api/setup-state":
			s.handleSetupState(w, r)
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

// handleLockBaseline pins an endpoint to one of its accepted versions, or
// releases it back to tracking the latest.
//
// This is the operation the product is named after and, until now, the only one
// with no way to reach it: SetLockedVersion had no route and no caller, so the
// dashboard's lock badge could never light up and "lock a known-good contract"
// was not something a user could do. Locking is distinct from promoting —
// accepting a new shape records a version, and pinning decides which accepted
// shape the endpoint is still held to.
//
// A version of 0 releases the pin. That is the same encoding GetBaseline reads:
// 0 means "track the latest", not "version zero", which cannot exist because
// versions are numbered from 1.
func (s *Server) handleLockBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.SetLockedVersion(req.Method, req.Path, req.Version); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hist, ok := s.store.GetHistory(req.Method, req.Path)
	if !ok {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}
	s.hub.Publish("baseline_locked", map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"version": req.Version,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hist)
}

// handleConfirmBaseline promotes a provisional version to a contract somebody
// stands behind.
//
// A version captured from live traffic is a guess about the API's contract, and
// while it stays unconfirmed Driftwood compares against that guess — so drift
// that predates Driftwood measures as MATCH and stays invisible. Confirming is
// the only thing that can distinguish the two, which is why it is a deliberate
// act with its own route rather than a side effect of anything else.
func (s *Server) handleConfirmBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.ConfirmBaseline(req.Method, req.Path, req.Version); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hist, ok := s.store.GetHistory(req.Method, req.Path)
	if !ok {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}
	s.hub.Publish("baseline_confirmed", map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"version": req.Version,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hist)
}

// isLoopbackRequest reports whether the request came from this machine.
//
// It is the trust boundary for retargeting, and the only one available: the
// dashboard has no login, so "who is asking" has to be answered by where the
// connection came from. An operator at their own browser is naming the API they
// want sniffed; the same string arriving from off-box is an instruction from
// someone who may not be entitled to give it, and Driftwood runs inside
// networks worth reaching.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// configPatch carries only the fields a request actually named, so that a field
// it left out can be told apart from a field it set to its zero value.
//
// Decoding straight into a types.ProxyConfig cannot make that distinction, and
// the difference is not academic: the dashboard's own save button posts
// target_url, auto_save_baseline, proxy_port and intercept_json, and says nothing
// about dev_mock_mode — so saving your proxy settings turned the mock mode off.
// A pointer per field is what makes "absent" representable.
type configPatch struct {
	TargetURL        *string `json:"target_url"`
	AutoSaveBaseline *bool   `json:"auto_save_baseline"`
	InterceptJSON    *bool   `json:"intercept_json"`
	DevMockMode      *bool   `json:"dev_mock_mode"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var patch configPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Start from what is running and overlay only what was named. The port is
		// never taken from the request at all: the listening socket is already
		// bound, so it is fixed for the life of the process. The dashboard has no
		// port field either — it sends a hardcoded 8787, which would otherwise be
		// persisted and adopted on the next start by anyone who had launched on a
		// different port.
		cfg := s.store.GetConfig()
		if patch.TargetURL != nil {
			cfg.TargetURL = *patch.TargetURL
		}
		if patch.AutoSaveBaseline != nil {
			cfg.AutoSaveBaseline = *patch.AutoSaveBaseline
		}
		if patch.InterceptJSON != nil {
			cfg.InterceptJSON = *patch.InterceptJSON
		}
		if patch.DevMockMode != nil {
			cfg.DevMockMode = *patch.DevMockMode
		}

		// Retargeted before it is persisted, so a target the proxy refuses is
		// never written down. Checking afterwards left the config panel
		// describing an endpoint that was not the one in use.
		if err := s.proxy.SetTarget(cfg.TargetURL, isLoopbackRequest(r)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.store.UpdateConfig(cfg); err != nil {
			// The change is live but will not survive a restart. Saying so is the
			// point: silently accepting a setting that does not persist is how the
			// config panel came to look like it worked.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.hub.Publish("config_updated", cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.GetConfig())
}

// handleSetupState answers whether this install has ever been configured, which
// is the dashboard's cue to offer first-run setup.
//
// A route of its own rather than a field on /api/config because it is a
// read-only fact about the install, not a setting: nothing posts it, and a field
// on a configuration endpoint is a field some client will eventually try to set.
// (handleConfig no longer stores a request body wholesale — see configPatch — so
// the older reason for splitting this out no longer applies, but this one does.)
func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"configured": s.store.IsConfigured()})
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
		// Sorted, because the output is a file a user keeps in their repository:
		// GetAllBaselines walks a map, so an unsorted export reordered itself
		// between runs and showed up as a diff with no change behind it. The
		// sort also makes the disambiguation below deterministic — a suffix
		// handed out in map order would move between endpoints from one export
		// to the next.
		sort.Slice(baselines, func(i, j int) bool {
			if baselines[i].Method != baselines[j].Method {
				return baselines[i].Method < baselines[j].Method
			}
			return baselines[i].Path < baselines[j].Path
		})

		used := make(map[string]bool, len(baselines))
		for _, b := range baselines {
			name := strings.Title(strings.ToLower(b.Method)) + cleanInterfaceName(b.Path) + "Response"
			// Two paths can normalize to one name — /api/users/{id} and
			// /api/users/id both give UsersId — and two interfaces of the same
			// name with different shapes is a file that does not compile.
			if used[name] {
				for n := 2; ; n++ {
					candidate := name + strconv.Itoa(n)
					if !used[candidate] {
						name = candidate
						break
					}
				}
			}
			used[name] = true

			ts := contract.GenerateTypeScriptInterfaces(name, b.Schema)
			sb.WriteString(ts)
			sb.WriteString("\n")
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"driftwood-contracts.d.ts\"")
	_, _ = w.Write([]byte(sb.String()))
}

// isIdentifierChar reports whether r may appear in a TypeScript identifier.
func isIdentifierChar(r rune) bool {
	return r == '_' || r == '$' ||
		('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

// cleanInterfaceName turns a URL path into the middle part of an interface name.
//
// Only identifier characters may survive: the result is interpolated into
// `export interface %s`, so anything else produces a .d.ts that does not parse.
// That was not true before — a parameter marker was stripped only in its
// Express form, so GET /api/users/{id} declared
// `export interface GetUsers{Id}Response`. Rather than add the second form to
// the list of characters to remove, this keeps what is legal and treats
// everything else (braces, colons, dots, percent escapes) as a word separator,
// which covers route syntaxes nobody has thought of yet.
func cleanInterfaceName(path string) string {
	var res strings.Builder
	for _, p := range strings.Split(path, "/") {
		if p == "" || p == "_driftwood" || p == "mock" || p == "api" {
			continue
		}
		for _, word := range strings.FieldsFunc(p, func(r rune) bool { return !isIdentifierChar(r) }) {
			res.WriteString(strings.Title(word))
		}
	}
	if res.Len() == 0 {
		return "Endpoint"
	}
	return res.String()
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
