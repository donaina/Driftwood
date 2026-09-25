package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

const thresholdsRoute = "/_driftwood/api/thresholds"

// thresholdsBody is the response shape this route returns.
type thresholdsBody struct {
	Preset string `json:"preset"`
	Floors []struct {
		Kind  string `json:"kind"`
		Floor string `json:"floor"`
	} `json:"floors"`
	Presets []struct {
		Name   string `json:"name"`
		Floors []struct {
			Kind  string `json:"kind"`
			Floor string `json:"floor"`
		} `json:"floors"`
	} `json:"presets"`
	UpdatedAt time.Time `json:"updated_at"`
}

func floorOf(t *testing.T, body thresholdsBody, kind types.DiffKind) string {
	t.Helper()
	for _, row := range body.Floors {
		if row.Kind == string(kind) {
			return row.Floor
		}
	}
	t.Fatalf("the response has no floor for %s", kind)
	return ""
}

// A project that has never saved a floor is answered with the one in force,
// which is the default. The dashboard renders this, so it has to be the truth
// rather than a placeholder.
func TestThresholdsReadAsTheDefaultBeforeAnythingIsSaved(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, thresholdsRoute, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body thresholdsBody
	readAndDecode(t, resp, &body)

	if body.Preset != types.PresetRecommended {
		t.Errorf("preset = %q, want %q", body.Preset, types.PresetRecommended)
	}
	if len(body.Floors) != len(types.DiffKinds) {
		t.Fatalf("the response carried %d floors, want one per kind (%d) — a row the server omits is a "+
			"row the dashboard has to guess at", len(body.Floors), len(types.DiffKinds))
	}
	for _, kind := range types.DiffKinds {
		if got := floorOf(t, body, kind); got != string(types.DefaultFloor) {
			t.Errorf("floor for %s = %q, want the default %q", kind, got, types.DefaultFloor)
		}
	}
	if !body.UpdatedAt.IsZero() {
		t.Error("a project that never saved a floor reported a save time")
	}

	// The presets travel with their floors, so the dashboard never expands a
	// preset name itself — a second definition of "strict" is how a screen comes
	// to offer a preset that is not the one it saves.
	if len(body.Presets) != len(types.PresetNames) {
		t.Fatalf("the response offered %d presets, want %d", len(body.Presets), len(types.PresetNames))
	}
	for _, preset := range body.Presets {
		floors, ok := types.FloorsForPreset(preset.Name)
		if !ok {
			t.Errorf("the response offered preset %q, which this build cannot expand", preset.Name)
			continue
		}
		if len(preset.Floors) != len(types.DiffKinds) {
			t.Errorf("preset %q carried %d floors, want %d", preset.Name, len(preset.Floors), len(types.DiffKinds))
		}
		for _, row := range preset.Floors {
			if want := floors[types.DiffKind(row.Kind)]; string(want) != row.Floor {
				t.Errorf("preset %q floor for %s = %q, want %q", preset.Name, row.Kind, row.Floor, want)
			}
		}
	}
}

// The floors the route saves are the floors the engine uses. This is the claim
// the whole phase rests on: before it, the screen collected severities and the
// store raised alerts on HasBreakingChanges || HasWarnings regardless.
func TestSavingThresholdsChangesWhatRaisesAnAlert(t *testing.T) {
	h := newHarness(t)
	project := h.store.ActiveProject()

	info := types.DiffDelta{JSONPath: "$.extra", Kind: types.KindAddedField, Severity: types.SeverityInfo}
	traffic := func(id string) types.CapturedTraffic {
		return types.CapturedTraffic{
			ID: id, Method: "GET", Path: "/api/users", StatusCode: 200, ContractStatus: "MATCH",
			Diff: &types.ContractDiff{Deltas: []types.DiffDelta{info}},
		}
	}

	// The default: an added field is recorded, not alerted on.
	if alert := h.store.AddTraffic(project, traffic("tr_before")); alert != nil {
		t.Fatal("an INFO delta alerted before anything was saved from the dashboard")
	}

	resp := h.postJSON(t, thresholdsRoute, `{"floors":[{"kind":"ADDED_FIELD","floor":"INFO"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var saved thresholdsBody
	readAndDecode(t, resp, &saved)

	// Answered with every kind, including the six the body did not mention, so
	// the dashboard can adopt the response wholesale.
	if len(saved.Floors) != len(types.DiffKinds) {
		t.Errorf("the save response carried %d floors, want %d", len(saved.Floors), len(types.DiffKinds))
	}
	if got := floorOf(t, saved, types.KindAddedField); got != string(types.SeverityInfo) {
		t.Errorf("ADDED_FIELD floor = %q, want INFO", got)
	}
	if got := floorOf(t, saved, types.KindRemovedField); got != string(types.DefaultFloor) {
		t.Errorf("a kind the body did not name came back as %q, want the default %q", got, types.DefaultFloor)
	}
	if saved.Preset != types.PresetCustom {
		t.Errorf("preset = %q, want %q for a config that is not one preset applied uniformly",
			saved.Preset, types.PresetCustom)
	}

	// The same delta now raises an alert, which is the end of the chain.
	alert := h.store.AddTraffic(project, traffic("tr_after"))
	if alert == nil {
		t.Fatal("an INFO delta raised no alert after the dashboard saved a floor of INFO — the route " +
			"stored a setting the engine does not read")
	}
	if len(alert.Diff.Deltas) != 1 || alert.Diff.Deltas[0].Severity != types.SeverityInfo {
		t.Error("the alert did not carry the delta that raised it")
	}
}

// And it survives a re-read, which is what the dashboard does on mount.
func TestSavedThresholdsAreReadBack(t *testing.T) {
	h := newHarness(t)

	h.postJSON(t, thresholdsRoute, `{"floors":[{"kind":"FORMAT_CHANGE","floor":"BREAKING"}]}`)

	var body thresholdsBody
	readAndDecode(t, h.do(t, http.MethodGet, thresholdsRoute, nil), &body)

	if got := floorOf(t, body, types.KindFormatChange); got != string(types.SeverityBreaking) {
		t.Errorf("FORMAT_CHANGE floor after re-read = %q, want BREAKING", got)
	}
	// Untouched kinds are still the default rather than having been reset to
	// whatever the single-row body implied.
	if got := floorOf(t, body, types.KindTypeMismatch); got != string(types.DefaultFloor) {
		t.Errorf("TYPE_MISMATCH floor = %q, want the default %q", got, types.DefaultFloor)
	}
}

// A floor names a kind and a severity the engine can act on, or it is refused.
// Stored, it would be a row that saves successfully and changes nothing.
func TestThresholdsRefuseWhatNothingCanActOn(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		name string
		body string
	}{
		{"a kind the engine never emits", `{"floors":[{"kind":"HEADER_CHANGE","floor":"INFO"}]}`},
		{"a severity this build cannot compute", `{"floors":[{"kind":"ADDED_FIELD","floor":"LOUD"}]}`},
		{"a body that is not JSON", `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.postJSON(t, thresholdsRoute, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if body := readAndDecode(t, resp, nil); body == "" {
				t.Error("the refusal carried no message; the caller is left with a status code and a guess")
			}
		})
	}

	// Nothing was stored by any of them.
	if got := h.store.GetThresholds(h.store.ActiveProject()).Preset(); got != types.PresetRecommended {
		t.Errorf("a refused request still changed the stored floors (preset = %q)", got)
	}
}

func TestThresholdsRejectAMethodTheyDoNotServe(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodDelete, thresholdsRoute, nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

// The floor is per project, and a named project that does not exist is refused
// rather than quietly answered with the active one's.
func TestThresholdsFollowTheNamedProject(t *testing.T) {
	h := newHarness(t)

	second, err := h.store.CreateProject("Second Client")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	resp := h.postJSON(t, thresholdsRoute+"?project="+second.ID, `{"floors":[{"kind":"ADDED_FIELD","floor":"INFO"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	readAndDecode(t, resp, nil)

	if got := h.store.GetThresholds(h.store.ActiveProject()).Preset(); got != types.PresetRecommended {
		t.Errorf("the active project's floors changed when another project's were saved (preset = %q)", got)
	}
	if got := h.store.GetThresholds(second.ID).FloorFor(types.KindAddedField); got != types.SeverityInfo {
		t.Errorf("the named project's ADDED_FIELD floor = %q, want INFO", got)
	}

	ghost := h.do(t, http.MethodGet, thresholdsRoute+"?project=no-such-project", nil)
	if ghost.StatusCode != http.StatusNotFound {
		t.Errorf("a ghost project answered %d, want 404 — naming a project that does not exist must not "+
			"fall back to the active one", ghost.StatusCode)
	}
}
