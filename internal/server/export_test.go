package server

import (
	"io"
	"regexp"
	"strings"
	"testing"
)

const exportTS = "/_driftwood/api/export/typescript"

// declaredNames returns every interface and type name the export declares.
func declaredNames(t *testing.T, body string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^export (?:interface|type) (\S+)`)
	var names []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		names = append(names, m[1])
	}
	return names
}

func fetchExport(t *testing.T, h *harness) string {
	t.Helper()
	resp := h.do(t, "GET", exportTS, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s = %d, want 200", exportTS, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading export body: %v", err)
	}
	return string(body)
}

var validTSIdentifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// The export is a .d.ts a user drops into their project, so every name it
// declares has to be a legal TypeScript identifier. A path template is not:
// cleanInterfaceName strips ':' but not '{}', so GET /api/users/{id} declares
// `export interface GetUsers{Id}Response`, which does not compile.
func TestExportTypeScript_DeclaresOnlyValidIdentifiers(t *testing.T) {
	h := newHarness(t)

	paths := []string{
		"/api/users",
		"/api/users/{id}",
		"/api/users/{user-id}",
		"/api/orders/:orderId",
		"/v1/2fa/verify",
	}
	for _, p := range paths {
		if _, err := h.store.SaveBaseline("GET", p, `{"id": 1, "username": "alex"}`); err != nil {
			t.Fatalf("SaveBaseline(GET %s): %v", p, err)
		}
	}

	body := fetchExport(t, h)
	names := declaredNames(t, body)
	if len(names) == 0 {
		t.Fatalf("the export declared no types:\n%s", body)
	}
	for _, n := range names {
		if !validTSIdentifier.MatchString(n) {
			t.Errorf("declared %q, which is not a valid TypeScript identifier\n\nfull export:\n%s", n, body)
		}
	}
}

// Two paths that normalize to the same name produce two `export interface` with
// one name. TypeScript merges identical declarations but rejects two that
// disagree, so a collision on endpoints with different shapes is a file that
// does not compile.
func TestExportTypeScript_DeclaresEachNameOnce(t *testing.T) {
	h := newHarness(t)

	// A path parameter and a literal segment with the same spelling normalize to
	// one name, and these two disagree about the shape.
	if _, err := h.store.SaveBaseline("GET", "/api/users/{id}", `{"id": 1, "kind": "one"}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	if _, err := h.store.SaveBaseline("GET", "/api/users/id", `{"name": "alex", "active": true}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	body := fetchExport(t, h)
	seen := map[string]int{}
	for _, n := range declaredNames(t, body) {
		seen[n]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("%q is declared %d times\n\nfull export:\n%s", name, count, body)
		}
	}
}

// Nested objects are named after the key alone, so two endpoints that both have
// a `user` object both declare `interface User`. Prefixing them with the
// endpoint's own interface name is what keeps the whole file collision-free.
func TestExportTypeScript_NestedNamesAreScopedToTheirEndpoint(t *testing.T) {
	h := newHarness(t)

	if _, err := h.store.SaveBaseline("GET", "/api/users", `{"user": {"id": 1, "name": "alex"}}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	if _, err := h.store.SaveBaseline("GET", "/api/orders", `{"user": {"reference": "abc", "total": 3}}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	body := fetchExport(t, h)
	seen := map[string]int{}
	for _, n := range declaredNames(t, body) {
		seen[n]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("%q is declared %d times: two endpoints with a same-named nested object collide\n\nfull export:\n%s",
				name, count, body)
		}
	}
	if _, ok := seen["User"]; ok {
		t.Errorf("a nested object was declared under the bare name User, which collides across endpoints\n\nfull export:\n%s", body)
	}
}

// An endpoint with no stored schema must still produce a well-formed export
// rather than a bare declaration with an empty name.
func TestExportTypeScript_NoBaselinesIsStillValid(t *testing.T) {
	h := newHarness(t)

	body := fetchExport(t, h)
	if !strings.Contains(body, "No baseline contracts locked yet") {
		t.Errorf("an empty export did not say so:\n%s", body)
	}
	if names := declaredNames(t, body); len(names) != 0 {
		t.Errorf("an empty export declared types: %v", names)
	}
}

// The export is a file a user commits, so the same contracts must produce the
// same bytes. GetAllBaselines walks a map, and the order of a Go map is
// randomized per iteration, so an unsorted export reorders itself between runs
// and shows up as a diff with nothing behind it. Five baselines are used
// because a smaller map can coincide with sorted order by chance, which would
// let an unsorted implementation pass here.
func TestExportTypeScript_IsDeterministic(t *testing.T) {
	h := newHarness(t)

	paths := []string{"/api/alpha", "/api/bravo", "/api/charlie", "/api/delta", "/api/echo"}
	for _, p := range paths {
		if _, err := h.store.SaveBaseline("GET", p, `{"id": 1}`); err != nil {
			t.Fatalf("SaveBaseline(GET %s): %v", p, err)
		}
	}

	body := fetchExport(t, h)
	names := declaredNames(t, body)

	want := []string{
		"GetAlphaResponse", "GetBravoResponse", "GetCharlieResponse",
		"GetDeltaResponse", "GetEchoResponse",
	}
	if len(names) != len(want) {
		t.Fatalf("declared %d names (%v), want %d", len(names), names, len(want))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("declaration order = %v, want %v — the export is not sorted", names, want)
		}
	}

	if again := fetchExport(t, h); again != body {
		t.Error("two exports of unchanged contracts produced different files")
	}
}
