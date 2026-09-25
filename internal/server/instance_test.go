package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donaina/driftwood/internal/proxy"
)

const (
	instanceRoute = "/_driftwood/api/instance"
	importRoute   = "/_driftwood/api/import"
)

// The handshake route is what stops the CLI handing a spec to whatever else
// happens to be on the port its hint names — a stale hint outlives the process
// that wrote it, and ports get reused. So the body has to identify the program,
// not merely answer 200.
func TestInstanceRouteIdentifiesDriftwood(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, instanceRoute, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", instanceRoute, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}

	var body struct {
		App           string `json:"app"`
		ActiveProject string `json:"active_project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.App != "driftwood" {
		t.Errorf(`app = %q, want "driftwood" — this is the value the CLI checks, and `+
			`anything else makes a running instance look like a stranger`, body.App)
	}
	if body.ActiveProject != h.store.ActiveProject() {
		t.Errorf("active_project = %q, want %q", body.ActiveProject, h.store.ActiveProject())
	}
}

func TestInstanceRouteRejectsNonGET(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, instanceRoute, `{}`)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST %s = %d, want %d", instanceRoute, resp.StatusCode, http.StatusMethodNotAllowed)
	}
	// And it never reached the backend: the control namespace is not proxied.
	if h.hitCount() != 0 {
		t.Errorf("the request reached the target API %d times", h.hitCount())
	}
}

// specFile writes a minimal but real OpenAPI document and returns its path.
//
// Two operations rather than one, and the second one is load-bearing: "the
// import publishes exactly one event" cannot be told apart from "one event per
// contract" by a document that holds a single contract. It is also the shape
// real imports have.
func specFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "openapi.json")
	doc := `{
	  "openapi": "3.0.0",
	  "info": {"title": "Widgets", "version": "1.0.0"},
	  "paths": {
	    "/widgets": {
	      "get": {
	        "operationId": "listWidgets",
	        "responses": {
	          "200": {
	            "description": "ok",
	            "content": {
	              "application/json": {
	                "schema": {
	                  "type": "object",
	                  "required": ["id"],
	                  "properties": {"id": {"type": "integer"}, "name": {"type": "string"}}
	                }
	              }
	            }
	          }
	        }
	      }
	    },
	    "/orders": {
	      "get": {
	        "operationId": "listOrders",
	        "responses": {
	          "200": {
	            "description": "ok",
	            "content": {
	              "application/json": {
	                "schema": {
	                  "type": "object",
	                  "required": ["reference"],
	                  "properties": {"reference": {"type": "string"}}
	                }
	              }
	            }
	          }
	        }
	      }
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatalf("writing the spec: %v", err)
	}
	return path
}

const specOperationCount = 2

// The point of the route: an import made through a running instance lands in
// the instance's own store, which is the state it serves from — so the next
// save cannot serialise it away.
//
// That is the whole bug. The CLI wrote the file behind a proxy holding its own
// copy of the document, and the proxy's next save replaced it; nothing failed
// and every request answered 200.
func TestImportThroughTheRunningInstanceLandsInTheStore(t *testing.T) {
	h := newHarness(t)

	resp := h.postJSON(t, importRoute, `{"spec": `+jsonString(t, specFile(t, t.TempDir()))+`}`)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s = %d: %s", importRoute, resp.StatusCode, body)
	}

	var body struct {
		Project  string `json:"project"`
		Imported []struct {
			Method      string `json:"method"`
			Path        string `json:"path"`
			OperationID string `json:"operation_id"`
		} `json:"imported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Project != h.store.ActiveProject() {
		t.Errorf("project = %q, want the active project %q", body.Project, h.store.ActiveProject())
	}
	if len(body.Imported) != specOperationCount {
		t.Fatalf("imported = %+v, want the %d operations in the spec", body.Imported, specOperationCount)
	}
	// The reply names what it filed, and the operation id travels with it: the
	// CLI prints these lines, and an id that went missing would have the
	// operator matching paths by eye against their own document.
	var listed bool
	for _, c := range body.Imported {
		if c.Method == "GET" && c.Path == "/widgets" {
			listed = true
			if c.OperationID != "listWidgets" {
				t.Errorf("operation_id = %q, want %q", c.OperationID, "listWidgets")
			}
		}
	}
	if !listed {
		t.Errorf("the reply omits GET /widgets: %+v", body.Imported)
	}

	// The claim that matters is the store's, not the response's: a reply listing
	// what it imported proves nothing about what it kept.
	baseline, ok := h.store.GetBaseline(body.Project, "GET", "/widgets")
	if !ok {
		t.Fatal("the imported contract is not in the store the instance serves from")
	}
	// And it is the declared schema, not one inferred back from the sample. The
	// spec marks `id` required and nothing else; inference over the generated
	// sample would not know that.
	if baseline.Schema == nil {
		t.Fatal("the imported baseline has no schema")
	}
	if len(baseline.Schema.RequiredKeys) != 1 || baseline.Schema.RequiredKeys[0] != "id" {
		t.Errorf("required keys = %v, want [id] as the document declared them", baseline.Schema.RequiredKeys)
	}
}

// An import through the API has to be visible to the dashboard that is already
// open on it, or the operator watches the import succeed and the list stay
// empty until they reload.
func TestImportPublishesOneEventForTheWholeImport(t *testing.T) {
	h := newHarness(t)

	// Through the real stream, so this asserts what a browser would receive
	// rather than what the server meant to send.
	stream := h.subscribe(t, h.store.ActiveProject())

	resp := h.postJSON(t, importRoute, `{"spec": `+jsonString(t, specFile(t, t.TempDir()))+`}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s = %d", importRoute, resp.StatusCode)
	}

	frame := stream.next(t, "the import")
	if frame["type"] != "baselines_imported" {
		t.Errorf("published %q, want baselines_imported", frame["type"])
	}

	// The payload is not decoration: the shell renders "N contracts filed under
	// X" from it, so a wrong count is a number on screen with nothing behind it.
	data, ok := frame["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("the event carries no data object: %v", frame["data"])
	}
	if data["project"] != h.store.ActiveProject() {
		t.Errorf("the event names project %v, want %q", data["project"], h.store.ActiveProject())
	}
	if data["imported"] != float64(specOperationCount) {
		t.Errorf("the event reports %v imported, want %d — the spec holds that many operations",
			data["imported"], specOperationCount)
	}

	// One event, not one per contract: the shell answers a baseline event by
	// refetching the list, so an import of forty operations would be forty
	// identical refetches. Nothing else may be waiting behind the first.
	if extra, ok := stream.pollWithin(250 * time.Millisecond); ok {
		t.Errorf("the import published a second event (%v)", extra["type"])
	}
}

func TestImportNamesTheProjectFromTheRequest(t *testing.T) {
	h := newHarness(t)
	other, err := h.store.CreateProject("Acme")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	resp := h.postJSON(t, importRoute, `{"spec": `+jsonString(t, specFile(t, t.TempDir()))+`, "project": `+jsonString(t, other.ID)+`}`)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s = %d: %s", importRoute, resp.StatusCode, body)
	}

	if _, ok := h.store.GetBaseline(other.ID, "GET", "/widgets"); !ok {
		t.Errorf("the contract was not filed under %q", other.ID)
	}
	if _, ok := h.store.GetBaseline(h.store.ActiveProject(), "GET", "/widgets"); ok {
		t.Errorf("the contract also landed under the active project %q", h.store.ActiveProject())
	}
}

// A project id that does not exist is refused rather than created, by the same
// function the CLI's own path uses — so the two channels cannot come to
// disagree about a typo.
func TestImportRefusesAnUnknownProject(t *testing.T) {
	h := newHarness(t)
	before, _ := h.store.ListProjects()

	resp := h.postJSON(t, importRoute, `{"spec": `+jsonString(t, specFile(t, t.TempDir()))+`, "project": "nope"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST %s = %d, want %d", importRoute, resp.StatusCode, http.StatusNotFound)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), h.store.ActiveProject()) {
		t.Errorf("the refusal does not name a project that does exist, so the caller can only guess: %s", body)
	}

	after, _ := h.store.ListProjects()
	if len(after) != len(before) {
		t.Errorf("a refused import changed the project list: %d -> %d", len(before), len(after))
	}
}

func TestImportRejectsAnEmptySpec(t *testing.T) {
	h := newHarness(t)

	for _, body := range []string{`{}`, `{"spec": ""}`, `{"spec": "   "}`} {
		resp := h.postJSON(t, importRoute, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST %s with %s = %d, want %d", importRoute, body, resp.StatusCode, http.StatusBadRequest)
		}
	}
}

// A spec that cannot be read is the caller's problem, not a server error, and
// the message has to say which path failed.
func TestImportReportsAnUnreadableSpec(t *testing.T) {
	h := newHarness(t)

	missing := filepath.Join(t.TempDir(), "absent.json")
	resp := h.postJSON(t, importRoute, `{"spec": `+jsonString(t, missing)+`}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST %s = %d, want %d", importRoute, resp.StatusCode, http.StatusBadRequest)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "absent.json") {
		t.Errorf("the error does not name the file that could not be read: %s", body)
	}
}

func TestImportRejectsNonPOST(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, importRoute, nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET %s = %d, want %d", importRoute, resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

// Loopback only, because the request names a file for the server to read and
// this control plane has no authentication of its own — a server bound to a
// wider interface would otherwise turn the route into a way to read any file
// the process can read and be told what the parser made of it.
//
// The check is the existing isLoopbackRequest, which also refuses a request
// that arrived through a proxy (the forwarded headers). This drives the route
// directly, since the harness itself always connects from loopback.
func TestImportRefusesANonLoopbackCaller(t *testing.T) {
	h := newHarness(t)
	spec := specFile(t, t.TempDir())

	for _, tc := range []struct {
		name       string
		remoteAddr string
		headers    map[string]string
	}{
		{name: "another host", remoteAddr: "203.0.113.7:51000"},
		{name: "through a proxy", remoteAddr: "127.0.0.1:51000", headers: map[string]string{"X-Forwarded-For": "203.0.113.7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, importRoute,
				strings.NewReader(`{"spec": `+jsonString(t, spec)+`}`))
			req.RemoteAddr = tc.remoteAddr
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			h.server.handleImportSpec(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
			if _, ok := h.store.GetBaseline(h.store.ActiveProject(), "GET", "/widgets"); ok {
				t.Error("a refused import still wrote a contract")
			}
		})
	}
}

// jsonString encodes s as a JSON string literal, so a path with a backslash or
// a quote in it (a Windows temp dir has both) does not produce a broken body.
func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshalling %q: %v", s, err)
	}
	return string(b)
}

// sseStream is one open event stream on the harness, with the frames it has
// received so far on a channel.
type sseStream struct {
	frames chan map[string]interface{}
}

// subscribe opens a stream through the router's own events route and returns
// once it is ready to receive.
//
// Readiness is the opening ping, not the response headers: the handler
// registers the subscriber before writing the ping, so a ping read back means a
// publish from this point on will be fanned out. Publishing straight after the
// GET races registration, and the failure looks like a dropped event rather
// than a test bug.
func (h *harness) subscribe(t *testing.T, projectID string) *sseStream {
	t.Helper()

	resp, err := http.Get(h.router.URL + proxy.ControlPrefix + "/events?project=" + projectID)
	if err != nil {
		t.Fatalf("opening the event stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	stream := &sseStream{frames: make(chan map[string]interface{}, 32)}
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		first := true
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			if first {
				close(ready)
				first = false
				continue
			}
			var frame map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
				continue
			}
			stream.frames <- frame
		}
	}()

	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("the event stream never sent its opening ping")
	}
	return stream
}

// next waits for a frame and fails if none arrives.
func (s *sseStream) next(t *testing.T, what string) map[string]interface{} {
	t.Helper()
	select {
	case frame := <-s.frames:
		return frame
	case <-time.After(3 * time.Second):
		t.Fatalf("nothing was published for %s", what)
		return nil
	}
}

// pollWithin reports whether another frame arrives within d: it is how a test
// asserts that something was NOT published.
//
// It waits rather than checking the channel once, because the publish happens
// on the handler's goroutine and the fan-out on the hub's — an immediate check
// would pass against an implementation that publishes one event per contract
// whenever the second one happened to be a moment behind. That the wait is a
// wall-clock one is the same compromise the rest of this suite makes: there is
// no clock abstraction in this repo, and the alternative is not asserting it.
func (s *sseStream) pollWithin(d time.Duration) (map[string]interface{}, bool) {
	select {
	case frame := <-s.frames:
		return frame, true
	case <-time.After(d):
		return nil, false
	}
}
