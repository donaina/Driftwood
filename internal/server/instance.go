package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/donaina/driftwood/internal/openapi"
	"github.com/donaina/driftwood/internal/storage"
)

/* The two routes that let a command line talk to a running instance instead of
   writing the file behind its back.

   `drift import` opens its own store, writes ~/.driftwood/baselines.json, and
   exits. The running proxy holds a separate view loaded at startup and
   serialises the whole document on its next save, so the import was erased by
   the next thing anyone did in the dashboard — measured, with every request
   answering 200 and nothing said. These routes are the other channel: the
   import asks the instance that owns the state to make the change, and the
   change is in memory before it is on disk.

   /api/instance exists so the caller can tell Driftwood from whatever else may
   be listening on the port a stale hint names. A 200 from a stranger is not a
   reason to hand it a spec. */

// handleInstance answers a caller asking whether an instance is here.
//
// Deliberately cheap and deliberately not project-scoped: it is a handshake, and
// a handshake that needed the caller to already know the project would not be
// one.
func (s *Server) handleInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		App           string `json:"app"`
		ActiveProject string `json:"active_project"`
	}{
		// The value the caller checks. Named "driftwood" rather than the module
		// path because it is read by a person debugging a stray port as often as
		// by the CLI, and "github.com/donaina/driftwood" answers a question
		// nobody asked.
		App:           "driftwood",
		ActiveProject: s.store.ActiveProject(),
	})
}

// handleImportSpec imports an OpenAPI document into the live store.
//
// The document is named, not sent, so that the parse lives in one place: the
// CLI has always accepted a path or a URL, and the server loads it through the
// same openapi entry point rather than a second reader of the same formats.
//
// Loopback only. The path form reads a file, and this server can be bound to a
// wider interface with no authentication of its own — the startup message says
// so — which would turn an import route into a way to read any file the process
// can read and be told what the parser made of it. The CLI that uses this runs
// on the same machine by definition, so the restriction costs it nothing.
func (s *Server) handleImportSpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "Importing is only accepted from this machine: the request names a file or a URL for the server to read, and this control plane is unauthenticated.", http.StatusForbidden)
		return
	}

	var req struct {
		Spec    string `json:"spec"`
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Spec) == "" {
		http.Error(w, "spec is required: a path or an http(s) URL to an OpenAPI document", http.StatusBadRequest)
		return
	}

	spec, err := openapi.LoadSpec(req.Spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The same resolution the CLI's file path uses, from the same function, so a
	// project id that does not exist is refused with the same sentence whichever
	// channel was asked.
	projectID, err := storage.ResolveImportProject(s.store, req.Project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	contracts, err := spec.ExtractContracts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := spec.ImportToStorage(projectID, s.store); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	/* One event for the whole import rather than one per contract. The shell
	   answers baseline_updated by re-reading the baseline list, so an import of
	   forty operations would be forty identical refetches; this says the thing
	   that actually happened, once. */
	s.hub.Publish(projectID, "baselines_imported", map[string]interface{}{
		"project":  projectID,
		"imported": len(contracts),
	})

	type importedContract struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		OperationID string `json:"operation_id"`
	}
	out := make([]importedContract, 0, len(contracts))
	for _, c := range contracts {
		out = append(out, importedContract{Method: c.Method, Path: c.Path, OperationID: c.OperationID})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Project  string             `json:"project"`
		Imported []importedContract `json:"imported"`
	}{Project: projectID, Imported: out})
}
