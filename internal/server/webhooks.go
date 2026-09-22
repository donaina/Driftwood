package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/donaina/driftwood/internal/netguard"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/pkg/types"
)

/* Where a project's alerts are delivered.

   A write is not refused for arriving off-box; it is checked against where it
   says to deliver. isLoopbackRequest becomes ParseAndValidate's allowPrivate
   argument, which is what the target route does — the same rule, applied to a
   destination instead of a backend.

   A blanket loopback gate was the alternative and is wrong twice. It would
   refuse a public Slack URL from a dashboard behind an authenticated reverse
   proxy, gaining nothing: the URL is public, and reaching it is the point. And
   it would refuse to point Driftwood at link-local metadata only by accident of
   where the request came from, rather than by the rule that exists to say so.
   What must not happen is a request that arrived over the wire aiming the
   deliverer at private infrastructure, and allowPrivate=false is precisely that
   statement.

   Deleting carries no URL, so there is nothing to check and nothing is checked,
   which makes it the one route here with no rule of its own. That is the same
   posture as /api/baselines/delete and /api/projects/delete: the control plane
   has no login, so anything that can reach it can already delete a project's
   contracts. Gating only this route would suggest a boundary the rest of the API
   does not have, and the operator would be wrong to rely on it.

   The secret is never in a response. webhookView has no field for it — the
   dashboard's question is "is one set", which has_secret answers, and a secret
   the browser has received has leaked into the browser cache, the DOM, and
   whatever screenshot the operator takes next. There is deliberately no reveal
   route: the operator who has lost their secret rotates it, which is the right
   answer anyway for a credential that has been lost. */

// webhookView is a config as the API reports it: everything except the secret.
type webhookView struct {
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	HasSecret bool      `json:"has_secret"`
	UpdatedAt time.Time `json:"updated_at"`
}

func viewOf(cfg types.WebhookConfig) webhookView {
	return webhookView{
		Kind:      cfg.Kind,
		URL:       cfg.URL,
		Enabled:   cfg.Enabled,
		HasSecret: cfg.Secret != "",
		UpdatedAt: cfg.UpdatedAt,
	}
}

// webhookList is the response body every one of these routes returns, so a
// caller that has just changed something is answered with the state it changed
// it to rather than having to ask again. Same shape as the project routes.
func (s *Server) webhookList(projectID string) []webhookView {
	configs := s.store.GetWebhooks(projectID)
	views := make([]webhookView, 0, len(configs))
	for _, cfg := range configs {
		views = append(views, viewOf(cfg))
	}
	return views
}

func (s *Server) writeWebhookList(w http.ResponseWriter, projectID string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.webhookList(projectID))
}

// handleWebhooks reads or writes one project's channel configs.
func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.writeWebhookList(w, projectID)

	case http.MethodPost:
		/* Pointers, so a field the request did not mention can be told apart
		   from one it set to its zero value.

		   configPatch documents the bug this prevents on the config route, where
		   saving proxy settings turned mock mode off because the dashboard
		   omitted a field and the decoder supplied its zero. Here the same
		   mistake would silently disable a channel, or worse: an omitted secret
		   decoded as "" reads as "clear the signing secret", so a dashboard that
		   only ever sends the URL would erase the secret every time somebody
		   pressed Save. Absent means leave it alone; "" means clear it. */
		var patch struct {
			Kind    string  `json:"kind"`
			URL     *string `json:"url"`
			Enabled *bool   `json:"enabled"`
			Secret  *string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !types.IsWebhookKind(patch.Kind) {
			http.Error(w, "unknown webhook kind", http.StatusBadRequest)
			return
		}

		// Start from what is stored and overlay only what was named, so a patch
		// carrying a single field does not blank the rest.
		cfg, _ := s.store.GetWebhook(projectID, patch.Kind)
		cfg.Kind = patch.Kind
		if patch.URL != nil {
			cfg.URL = *patch.URL
		}
		if patch.Enabled != nil {
			cfg.Enabled = *patch.Enabled
		}
		if patch.Secret != nil {
			cfg.Secret = *patch.Secret
		}

		// A channel with no URL cannot deliver anything, and enabling one would
		// put a channel in the dashboard that is on and points nowhere. Refused
		// rather than stored, because the dashboard's card would otherwise show
		// "enabled" for a setting that does nothing.
		if cfg.Enabled && cfg.URL == "" {
			http.Error(w, "a channel needs a URL before it can be enabled", http.StatusBadRequest)
			return
		}

		if cfg.URL != "" {
			/* Validated here rather than in the store, because this is the only
			   place that knows where the request came from — the same split
			   SetProjectTarget documents.

			   A refused URL is 400 and a URL that was accepted but not written
			   down is 500. Reporting the second as the first tells the operator
			   to fix a URL that was fine, which is the bug this idiom already
			   exists to prevent on the target route. */
			if _, err := netguard.ParseAndValidate(cfg.URL, isLoopbackRequest(r)); err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, netguard.ErrInvalidTarget) {
					status = http.StatusBadRequest
				}
				http.Error(w, err.Error(), status)
				return
			}
		}

		if err := s.store.SetWebhook(projectID, cfg); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, storage.ErrNoSuchProject) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}

		s.hub.Publish(projectID, "webhook_updated", s.webhookList(projectID))
		s.writeWebhookList(w, projectID)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeleteWebhook removes one channel's config.
//
// A POST rather than a DELETE with a path segment, matching
// /api/baselines/delete and /api/projects/delete: the router is a flat switch on
// the path, and a path carrying the kind would mean parsing it back out.
func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
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
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !types.IsWebhookKind(req.Kind) {
		http.Error(w, "unknown webhook kind", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteWebhook(projectID, req.Kind); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNoSuchProject) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	s.hub.Publish(projectID, "webhook_updated", s.webhookList(projectID))
	s.writeWebhookList(w, projectID)
}
