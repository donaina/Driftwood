package server

import (
	"net/http"
	"testing"

	"github.com/donaina/driftwood/pkg/types"
)

// seedVersions saves each payload as a version of one endpoint, in order, so
// version N is the Nth payload. Returns the endpoint every case here reads.
func seedVersions(t *testing.T, h *harness, payloads ...string) (method, path string) {
	t.Helper()
	method, path = http.MethodGet, "/api/users"
	for _, payload := range payloads {
		if _, err := h.store.SaveBaseline(h.store.ActiveProject(), method, path, payload); err != nil {
			t.Fatalf("SaveBaseline(%s): %v", payload, err)
		}
	}
	return method, path
}

// The panel this route exists for: two versions are selected, and the deltas
// between them are the answer. Before this route the dashboard said the feature
// "would require enhanced backend API" — which had stopped being true the moment
// the proxy grew a diff engine, since the schemas were already on the wire.
func TestHistoryDiffReportsWhatChangedBetweenTwoVersions(t *testing.T) {
	h := newHarness(t)
	seedVersions(t, h, `{"id":1,"name":"a"}`, `{"id":1}`)

	resp := h.do(t, http.MethodGet, "/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=1&to=2", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got types.ContractDiff
	decodeBody(t, resp, &got)

	if len(got.Deltas) != 1 {
		t.Fatalf("got %d deltas, want the one field that went missing: %+v", len(got.Deltas), got.Deltas)
	}
	d := got.Deltas[0]
	if d.JSONPath != "$.name" {
		t.Errorf("json_path = %q, want %q", d.JSONPath, "$.name")
	}
	if d.Kind != types.KindRemovedField {
		t.Errorf("kind = %q, want %q", d.Kind, types.KindRemovedField)
	}
	// An inferred baseline marks every key it has seen as required, so a field
	// that was there and is not is a break rather than a warning. That is the
	// engine's reading of its own baseline and this route must not soften it.
	if d.Severity != types.SeverityBreaking {
		t.Errorf("severity = %q, want %q", d.Severity, types.SeverityBreaking)
	}
	if !got.HasBreakingChanges {
		t.Error("has_breaking_changes is false beside a BREAKING delta; the two must agree")
	}
	if got.HasWarnings {
		t.Error("has_warnings is true with no WARNING delta")
	}
}

// Direction is not decoration. Comparing the same two versions the other way
// round reports the same field as added, and at INFO rather than BREAKING —
// which is why the dashboard orders its pair oldest-first rather than in click
// order, and why this route refuses to guess which end the caller meant.
func TestHistoryDiffDirectionDecidesAddedFromRemoved(t *testing.T) {
	h := newHarness(t)
	seedVersions(t, h, `{"id":1,"name":"a"}`, `{"id":1}`)

	resp := h.do(t, http.MethodGet, "/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=2&to=1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got types.ContractDiff
	decodeBody(t, resp, &got)

	if len(got.Deltas) != 1 {
		t.Fatalf("got %d deltas, want 1", len(got.Deltas))
	}
	if k := got.Deltas[0].Kind; k != types.KindAddedField {
		t.Errorf("kind = %q, want %q when the newer version is the baseline", k, types.KindAddedField)
	}
	if s := got.Deltas[0].Severity; s != types.SeverityInfo {
		t.Errorf("severity = %q, want %q", s, types.SeverityInfo)
	}
	if got.HasBreakingChanges {
		t.Error("a version gaining a field reported as a break; it is the one direction that is not")
	}
}

// Two versions of the same shape diff to nothing, and that is an answer rather
// than an empty response: the dashboard renders it as "no structural change",
// which it can only do if the route says so with 200 and an empty delta list.
func TestHistoryDiffOfIdenticalVersionsIsEmptyNotAnError(t *testing.T) {
	h := newHarness(t)
	seedVersions(t, h, `{"id":1,"name":"a"}`, `{"id":1,"name":"b"}`)

	resp := h.do(t, http.MethodGet, "/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=1&to=2", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got types.ContractDiff
	decodeBody(t, resp, &got)
	if len(got.Deltas) != 0 {
		t.Errorf("got %d deltas for two versions of one shape, want none: %+v", len(got.Deltas), got.Deltas)
	}
	if got.HasBreakingChanges || got.HasWarnings {
		t.Error("a same-shape pair flagged as drift")
	}
}

// The four ways to ask a question this route cannot answer. Each is asserted on
// the status and not on the body, because what matters here is that a caller can
// tell the mistakes apart — a 404 for a malformed query would send someone
// looking for an endpoint that is right there.
func TestHistoryDiffRefusesWhatItCannotAnswer(t *testing.T) {
	h := newHarness(t)
	seedVersions(t, h, `{"id":1}`, `{"id":2}`)

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"a version that does not exist", http.MethodGet,
			"/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=1&to=9", http.StatusNotFound},
		{"an endpoint that does not exist", http.MethodGet,
			"/_driftwood/api/histories/diff?method=GET&path=%2Fnope&from=1&to=2", http.StatusNotFound},
		{"a from that is not a number", http.MethodGet,
			"/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=x&to=2", http.StatusBadRequest},
		{"no to at all", http.MethodGet,
			"/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=1", http.StatusBadRequest},
		{"a write to a read", http.MethodPost,
			"/_driftwood/api/histories/diff?method=GET&path=%2Fapi%2Fusers&from=1&to=2", http.StatusMethodNotAllowed},
		{"a project that does not exist", http.MethodGet,
			"/_driftwood/api/histories/diff?project=ghost&method=GET&path=%2Fapi%2Fusers&from=1&to=2", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.do(t, tc.method, tc.path, nil)
			if resp.StatusCode != tc.want {
				t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
			}
		})
	}
}

// The dashboard's HistoryItem declares `schema` on every version, and this route
// reads the same field from the store. Pinning it here is what keeps that
// declaration from becoming another field the type claims and the API does not
// send — which is the bug the comment above that interface was written about.
func TestHistoriesCarryEachVersionsSchema(t *testing.T) {
	h := newHarness(t)
	seedVersions(t, h, `{"id":1,"name":"a"}`, `{"id":2}`)

	resp := h.do(t, http.MethodGet, "/_driftwood/api/histories", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got []struct {
		Versions []struct {
			Version int                   `json:"version"`
			Schema  *types.JSONSchemaNode `json:"schema"`
		} `json:"versions"`
	}
	decodeBody(t, resp, &got)

	if len(got) != 1 || len(got[0].Versions) != 2 {
		t.Fatalf("got %d endpoints; want one endpoint with two versions", len(got))
	}
	for _, v := range got[0].Versions {
		if v.Schema == nil {
			t.Fatalf("v%d carries no schema; the diff route has nothing to compare", v.Version)
		}
		if v.Schema.Type != types.TypeObject {
			t.Errorf("v%d schema type = %q, want %q", v.Version, v.Schema.Type, types.TypeObject)
		}
	}
}
