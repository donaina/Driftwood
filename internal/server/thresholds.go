package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

/* What raises an alert, per project.

   This route is the whole backend the Thresholds screen never had. Before it,
   CustomAlertThresholds.tsx collected five severities that were read by nothing:
   saveThresholdConfig toasted "Custom alert thresholds have been saved." over an
   empty function body, and the store's decision to raise an alert was the
   literal HasBreakingChanges || HasWarnings. The screen configured one thing and
   the engine did another, which is this product's one unforgivable shape of bug
   — the dashboard saying something the measurement does not.

   The predicate now lives on the config and is read in store.AddTraffic, which
   is the only place that decides whether an alert exists. Nothing here restates
   it: this file validates and stores, and the store answers the question when a
   request arrives.

   The floors are per delta kind, because the engine emits seven and the old
   screen's five rows did not partition them — two of its rows configured the
   same kind twice, and "Header Changes" configured no kind at all, since headers
   are recorded and never diffed.

   Floors travel as an array rather than an object, here and in the response.
   That is not a style choice: encoding/json sorts map keys, so an object would
   arrive with the kinds in alphabetical order and the dashboard would render a
   config screen whose rows are in no order a person reads. An array carries
   types.DiffKinds' order through the wire, which keeps the one definition of
   that order in Go rather than restating it in TypeScript. */

// thresholdsFloorView is one row of the table.
type thresholdsFloorView struct {
	Kind  types.DiffKind     `json:"kind"`
	Floor types.DiffSeverity `json:"floor"`
}

// thresholdsPresetView is one preset as the dashboard offers it.
//
// The floors come from the server so the dashboard never has to know what
// "strict" means. A preset name expanded in TypeScript would be a second
// definition of the same three words, and the two would drift the first time
// one of them changed — which is how a screen comes to offer a preset that is
// not the preset it saves.
type thresholdsPresetView struct {
	Name   string                `json:"name"`
	Floors []thresholdsFloorView `json:"floors"`
}

// thresholdsView is a config as the API reports it.
type thresholdsView struct {
	Preset    string                 `json:"preset"`
	Floors    []thresholdsFloorView  `json:"floors"`
	Presets   []thresholdsPresetView `json:"presets"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// floorsOf returns the rows in the engine's own kind order.
//
// Complete, one entry per kind, rather than the sparse map that is stored. The
// dashboard renders a row per kind and has to show each row's effective value,
// so a missing kind would leave it rendering either nothing or its own guess at
// the default — and a default restated in TypeScript is the second copy this
// route exists to avoid.
func floorsOf(cfg types.ThresholdConfig) []thresholdsFloorView {
	floors := make([]thresholdsFloorView, 0, len(types.DiffKinds))
	for _, kind := range types.DiffKinds {
		floors = append(floors, thresholdsFloorView{Kind: kind, Floor: cfg.FloorFor(kind)})
	}
	return floors
}

func viewOfThresholds(cfg types.ThresholdConfig) thresholdsView {
	presets := make([]thresholdsPresetView, 0, len(types.PresetNames))
	for _, name := range types.PresetNames {
		set, ok := types.FloorsForPreset(name)
		if !ok {
			continue
		}
		presets = append(presets, thresholdsPresetView{Name: name, Floors: floorsOf(types.ThresholdConfig{Floors: set})})
	}

	return thresholdsView{
		// Derived from the floors, never stored: see types.ThresholdConfig.Preset,
		// which is the one place that decides what the name is.
		Preset:    cfg.Preset(),
		Floors:    floorsOf(cfg),
		Presets:   presets,
		UpdatedAt: cfg.UpdatedAt,
	}
}

// decodeFloors turns the request's rows into the map the store holds.
//
// A repeated kind takes its last value rather than being refused. Two rows for
// one kind is a malformed body either way, and the alternative — an error — would
// be reported against a request whose meaning is perfectly clear.
func decodeFloors(rows []thresholdsFloorView) map[types.DiffKind]types.DiffSeverity {
	floors := make(map[types.DiffKind]types.DiffSeverity, len(rows))
	for _, row := range rows {
		floors[row.Kind] = row.Floor
	}
	return floors
}

func (s *Server) writeThresholds(w http.ResponseWriter, projectID string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(viewOfThresholds(s.store.GetThresholds(projectID)))
}

// handleThresholds reads or writes one project's alert floor.
//
// GET answers the floor in force, which for a project that has never saved one
// is the default. That is not a placeholder: the default is what AddTraffic
// applies, so a dashboard rendering it is rendering the truth rather than a
// guess about what the server might do.
func (s *Server) handleThresholds(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectFor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.writeThresholds(w, projectID)

	case http.MethodPost:
		// The floors are the whole request, so a body naming a kind or a severity
		// this build cannot act on is refused rather than stored. Storing one would
		// put a row in the dashboard that saves successfully and changes nothing.
		var body struct {
			Floors []thresholdsFloorView `json:"floors"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		floors := decodeFloors(body.Floors)
		if err := types.ValidateFloors(floors); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cfg, err := s.store.SetThresholds(projectID, floors)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		/* Announced on the same channel as every other project-scoped change.
		   Nothing in the browser listens for this yet — the view that wrote it
		   already has the answer, and a second tab showing the same project is the
		   case it is for. It is published rather than left out because the
		   alternative is discovering later that a screen shows a stale floor with
		   no way for it to hear otherwise. */
		s.hub.Publish(projectID, "thresholds_updated", viewOfThresholds(cfg))

		// The saved config, not a bare 200, for the reason webhookList gives: a
		// caller that has just changed something is answered with the state it
		// changed it to rather than having to ask again. Every kind comes back, so
		// a body that named three is answered with all seven and the dashboard can
		// adopt it wholesale.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(viewOfThresholds(cfg))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
