package server

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/donaina/driftwood/internal/webhook"
	"github.com/donaina/driftwood/pkg/types"
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

	// deliveries is what the server needs from the alert deliverer, and is
	// deliberately the narrow interface rather than the *webhook.Deliverer: the
	// API serves the records and can ask for one test send, and the enqueue side
	// is still not in reach from here. Never nil — see NewServer.
	//
	// The test route is why this is no longer only a log. The server can now
	// cause an outbound request, which is worth being explicit about: the
	// destination is operator-configured rather than caller-chosen, which is what
	// keeps a "send a test" button from being an SSRF primitive.
	deliveries webhook.Deliveries
}

// NewServer assembles the control plane.
//
// deliveries is a sixth parameter rather than a field set afterwards because a
// Server that forgets it would answer the delivery-records route with a nil
// dereference, and the route is not on the proxy path — nothing about a request
// through the proxy would have exercised it. A parameter makes the omission a
// compile error at all three call sites instead.
func NewServer(store *storage.Store, hub *events.Hub, prx *proxy.Proxy, mockCtrl *mock.MockController, siteHandler http.Handler, deliveries webhook.Deliveries) *Server {
	if deliveries == nil {
		// A server built without a deliverer answers the records route with an
		// empty list rather than panicking, and the test route with a 500. Both
		// are the honest answer: nothing has been delivered because nothing is
		// delivering, and a server that cannot send has nothing to report about
		// whether a send would work.
		deliveries = webhook.NoDeliveries{}
	}
	return &Server{
		store:      store,
		hub:        hub,
		proxy:      prx,
		mockCtrl:   mockCtrl,
		site:       siteHandler,
		deliveries: deliveries,
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
			projectID, err := s.projectFor(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			s.hub.SSEHandler(w, r, projectID)
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
		case proxy.ControlPrefix + "/api/projects":
			s.handleProjects(w, r)
		case proxy.ControlPrefix + "/api/projects/create":
			s.handleCreateProject(w, r)
		case proxy.ControlPrefix + "/api/projects/update":
			s.handleUpdateProject(w, r)
		case proxy.ControlPrefix + "/api/projects/delete":
			s.handleDeleteProject(w, r)
		case proxy.ControlPrefix + "/api/projects/active":
			s.handleSetActiveProject(w, r)
		case proxy.ControlPrefix + "/api/webhooks":
			s.handleWebhooks(w, r)
		case proxy.ControlPrefix + "/api/webhooks/delete":
			s.handleDeleteWebhook(w, r)
		case proxy.ControlPrefix + "/api/webhooks/deliveries":
			s.handleDeliveries(w, r)
		case proxy.ControlPrefix + "/api/webhooks/test":
			s.handleTestWebhook(w, r)
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
	// X-Driftwood-Target is deliberately not advertised here. A header letting a
	// caller choose which operator-configured backend it reaches would be
	// privilege escalation against a posture where loopback is the only trust
	// boundary, and there is no identity to check it against. Switching project
	// is a dashboard action (POST /api/projects/active), not a per-request one.
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// errNoSuchProject is projectFor's refusal, and aliases the store's own sentinel
// so a handler has one thing to test rather than two names for the same
// condition — a request naming a project that does not exist and a body naming
// one are the same mistake and deserve the same answer.
var errNoSuchProject = storage.ErrNoSuchProject

/*
projectFor resolves which project a request is about.

	An explicit ?project= wins, so a caller can read one client's baselines while
	another is active — which is what an export or a script needs. Otherwise the
	active project answers, so the ordinary case needs no parameter and the
	dashboard's views follow its switcher.

	A named project that does not exist is an error rather than a fallback to the
	active one. Falling back would answer a question about project B with project
	A's data under no indication that it had done so — the quiet misattribution
	this codebase is strict about. It would also mean a typo in a script silently
	read the wrong client's contracts.

	This is applied uniformly to reads and to writes. It was tempting to honour the
	parameter on reads only, since reads are the ones a caller might legitimately
	want cross-project — but then a caller that named a project it was not looking
	at would read that project and write the active one, which is the same
	mismatch arrived at from the other side. One rule, and the two cannot
	disagree.
*/
func (s *Server) projectFor(r *http.Request) (string, error) {
	id := r.URL.Query().Get("project")
	if id == "" {
		return s.store.ActiveProject(), nil
	}
	if !s.store.ProjectExists(id) {
		return "", fmt.Errorf("%w: %q", errNoSuchProject, id)
	}
	return id, nil
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	traffics := s.store.GetTraffics(projectID, 100)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(traffics)
}

func (s *Server) handleClearTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.store.ClearTraffic(projectID)
	s.hub.Publish(projectID, "traffic_cleared", nil)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleBaselines(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

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

		cb, err := s.store.SaveBaseline(projectID, req.Method, req.Path, req.Payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.hub.Publish(projectID, "baseline_updated", cb)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cb)
		return
	}

	baselines := s.store.GetAllBaselines(projectID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(baselines)
}

func (s *Server) handleDeleteBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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
	// The clear is done in memory either way; a failed write means it will not
	// survive a restart, and the contract will be back. Same reasoning as the
	// config route above: an operation reported as done that was not persisted
	// coming undone on the next restart is the failure this reports instead.
	if err := s.store.DeleteBaseline(projectID, req.Method, req.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.hub.Publish(projectID, "baseline_deleted", req)
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
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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

	if err := s.store.SetLockedVersion(projectID, req.Method, req.Path, req.Version); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hist, ok := s.store.GetHistory(projectID, req.Method, req.Path)
	if !ok {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}
	s.hub.Publish(projectID, "baseline_locked", map[string]interface{}{
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
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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

	if err := s.store.ConfirmBaseline(projectID, req.Method, req.Path, req.Version); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hist, ok := s.store.GetHistory(projectID, req.Method, req.Path)
	if !ok {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}
	s.hub.Publish(projectID, "baseline_confirmed", map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"version": req.Version,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hist)
}

// forwardedHeaders are the headers a reverse proxy sets to describe the client
// it is speaking for. Caddy, nginx, Cloudflare and the rest all set at least one
// of them, and none is set by a client talking to this process directly.
var forwardedHeaders = []string{
	"Forwarded",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Real-Ip",
}

// isLoopbackRequest reports whether the request came from this machine.
//
// It is the trust boundary for retargeting, and the only one available: the
// dashboard has no login, so "who is asking" has to be answered by where the
// connection came from. An operator at their own browser is naming the API they
// want sniffed; the same string arriving from off-box is an instruction from
// someone who may not be entitled to give it, and Driftwood runs inside
// networks worth reaching.
//
// A reverse proxy inverts that answer. It opens its own connection to this
// process from 127.0.0.1 whatever the real client's address was, so RemoteAddr
// alone cannot tell an operator at their own browser from the whole internet:
// every request arriving through Caddy is classified loopback and the
// private-target refusal this guards never fires. The proxy's forwarded header
// is its own evidence that something stood in front of the request, so the
// presence of one withdraws the trust rather than granting it.
//
// That direction is the point. A client that forges a forwarded header on a
// direct connection only loses access, never gains any, so the check fails
// closed and cannot be talked around. A request carrying no such header that
// arrives from loopback is still trusted, which is what keeps an SSH tunnel —
// terminating on this machine, adding no headers — working unchanged.
func isLoopbackRequest(r *http.Request) bool {
	for _, h := range forwardedHeaders {
		if r.Header.Get(h) != "" {
			return false
		}
	}
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

		// Retargeted first, and before anything is read back or persisted, so a
		// target the proxy refuses is never written down. Checking afterwards left
		// the config panel describing an endpoint that was not the one in use.
		//
		// This route configures the *active project's* target, which for the
		// single-project install this has always been is exactly what it did
		// before. Per-project targets are set from the project routes instead.
		if patch.TargetURL != nil {
			if err := s.proxy.SetProjectTarget(
				s.store.ActiveProject(), *patch.TargetURL, isLoopbackRequest(r),
			); err != nil {
				// A refused target is the client's problem; a target that was
				// accepted but not written down is ours. Reporting the second as
				// the first tells the operator to fix a URL that was fine.
				status := http.StatusInternalServerError
				if errors.Is(err, proxy.ErrInvalidTarget) {
					status = http.StatusBadRequest
				}
				http.Error(w, err.Error(), status)
				return
			}
		}

		// Start from what is running and overlay only what was named. The port is
		// never taken from the request at all: the listening socket is already
		// bound, so it is fixed for the life of the process. The dashboard has no
		// port field either — it sends a hardcoded 8787, which would otherwise be
		// persisted and adopted on the next start by anyone who had launched on a
		// different port.
		//
		// Read after the retarget rather than before, so the target carried into
		// UpdateConfig is the one the proxy accepted rather than the one the
		// request asked for.
		cfg := s.store.GetConfig()
		if patch.AutoSaveBaseline != nil {
			cfg.AutoSaveBaseline = *patch.AutoSaveBaseline
		}
		if patch.InterceptJSON != nil {
			cfg.InterceptJSON = *patch.InterceptJSON
		}
		if patch.DevMockMode != nil {
			cfg.DevMockMode = *patch.DevMockMode
		}

		if err := s.store.UpdateConfig(cfg); err != nil {
			// The change is live but will not survive a restart. Saying so is the
			// point: silently accepting a setting that does not persist is how the
			// config panel came to look like it worked.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		/* Install-level, so every subscriber hears it.

		   The config is both: proxy_port and the booleans belong to the install,
		   while target_url is the active project's. Publishing it to the active
		   project's subscribers alone would leave another project's open dashboard
		   holding stale booleans, and it is the booleans — auto_save_baseline,
		   intercept_json, dev_mock_mode — that change what the proxy does to every
		   project's traffic. */
		s.hub.Publish("", "config_updated", cfg)
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
// handleSetupState tells the dashboard whether to open the setup wizard, whose
// first question is "where is your API running?".
//
// So what it reports is whether that question is answered for the project now
// in force, not whether anyone has ever saved a setting. IsConfigured alone
// answers the second question, and it says yes to the install that was pointed
// at a backend once — which is exactly the install that can go on to create a
// second project with no backend, and had no way to be asked about it. That
// gap is what let a targetless active project reach startup with nothing
// prompting for a target; the wizard was the one surface that asks, and it was
// switched off by a flag that stopped meaning the right thing the moment a
// second project existed.
//
// The wizard is dismissible, so an operator who arrives here mid-life with a
// project they are not ready to point anywhere can close it and carry on.
func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	url, _, ok := s.store.ProjectTarget(s.store.ActiveProject())
	configured := s.store.IsConfigured() && ok && strings.TrimSpace(url) != ""

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"configured": configured})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	alerts := s.store.GetAlerts(projectID, 50)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(alerts)
}

func (s *Server) handleHistories(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	histories := s.store.GetAllHistories(projectID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(histories)
}

func (s *Server) handleExportTypeScript(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	baselines := s.store.GetAllBaselines(projectID)
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

// projectList is the shape every project route answers with: which project is
// active, and the projects themselves in their stored order.
//
// One shape for all of them rather than each returning only what it changed, so
// the dashboard can replace its whole view of the project list from any response
// instead of patching its own copy and hoping the two agree.
type projectList struct {
	Active   string          `json:"active"`
	Projects []types.Project `json:"projects"`
}

func (s *Server) projectList() projectList {
	projects, active := s.store.ListProjects()
	return projectList{Active: active, Projects: projects}
}

// announceProjects tells every subscriber the project list changed.
//
// Install-level, so it reaches dashboards watching any project: a project
// appearing, disappearing or being renamed is news to all of them, and a client
// watching a project another operator just deleted needs to hear about it rather
// than sit on a stream that will never carry another event.
func (s *Server) announceProjects() {
	s.hub.Publish("", "projects_updated", s.projectList())
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.projectList())
}

// handleCreateProject adds a project, optionally with its target already set.
//
// The target is validated before the project is made. Creating first and undoing
// on failure would leave a window in which a project exists holding a target
// nothing checked, and a rollback path that has to be correct in the one place a
// mistake means a client pointing at nothing.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name      string `json:"name"`
		TargetURL string `json:"target_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The same loopback rule as retargeting an existing project: a private
	// backend can only be named from this machine. Checking it here rather than
	// in the store keeps the decision where the request's origin is knowable.
	allowPrivate := isLoopbackRequest(r)
	if req.TargetURL != "" {
		if err := s.proxy.ValidateTarget(req.TargetURL, allowPrivate); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	project, err := s.store.CreateProject(req.Name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrTooManyProjects) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}

	if req.TargetURL != "" {
		if err := s.proxy.SetProjectTarget(project.ID, req.TargetURL, allowPrivate); err != nil {
			// The project exists and will keep its unset target, which is a state
			// it is allowed to be in and which answers 502 rather than borrowing
			// another project's backend. Reported rather than hidden.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Re-read rather than returning the struct CreateProject produced: the target
	// was written into the store's copy, so the local one still says the project
	// has no backend. Returning it would tell a dashboard that a project it just
	// configured points at nothing.
	if current, ok := s.store.GetProject(project.ID); ok {
		project = &current
	}

	s.announceProjects()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(project)
}

// handleUpdateProject renames a project, retargets it, or both.
//
// Retargeting goes through the proxy so the SSRF rule applies and the routing
// snapshot is rebuilt — a target written straight into the store would be one the
// proxy does not know about until something else happened to change routing.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		// A pointer so that an omitted target and one sent as "" are
		// distinguishable. They are not the same request: omitting it means "leave
		// the target alone", and an empty string is a target, just not one that
		// parses.
		TargetURL *string `json:"target_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.store.ProjectExists(req.ID) {
		http.Error(w, fmt.Sprintf("%v: %q", storage.ErrNoSuchProject, req.ID), http.StatusNotFound)
		return
	}

	if req.Name != "" {
		if err := s.store.RenameProject(req.ID, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if req.TargetURL != nil {
		allowPrivate := isLoopbackRequest(r)
		err := s.proxy.SetProjectTarget(req.ID, *req.TargetURL, allowPrivate)
		switch {
		case err == nil:
		case errors.Is(err, proxy.ErrInvalidTarget):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.announceProjects()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.projectList())
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteProject(req.ID); err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, storage.ErrNoSuchProject):
			status = http.StatusNotFound
		case errors.Is(err, storage.ErrLastProject):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}

	s.announceProjects()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.projectList())
}

// handleSetActiveProject switches which project the proxy serves.
//
// The proxy is not told directly. The store records the change and bumps its
// routing generation, and the next request that finds its snapshot out of date
// rebuilds it — so a switch cannot be half-applied by a handler that forgot to
// mention it to one of the two.
func (s *Server) handleSetActiveProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.SetActiveProject(req.ID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrNoSuchProject) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	s.announceProjects()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.projectList())
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
		// Install-level: the mock simulator stands in for every target at once,
		// so a mode change is not any one project's news.
		s.hub.Publish("", "mock_mode_changed", map[string]string{"mode": req.Mode})
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
