package site

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withoutDisk points the on-disk lookup at a directory that does not exist, so
// the embedded copy is the only source a test can be reading from.
//
// This is not cosmetic. The files in site/dist are build output and only the
// placeholder is committed, so whether they are present depends on whether the
// machine running the test has run a Vite build — which would make these tests
// pass in one checkout and fail in another. The embed is compiled in, so it is
// the same everywhere.
func withoutDisk(t *testing.T) {
	t.Helper()
	dd := distDir
	distDir = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { distDir = dd })
}

func get(target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	ServeSite(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestServeSite_ServesTheLandingPageFromTheEmbed(t *testing.T) {
	withoutDisk(t)

	rec := get("/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Error("the page served from the embed does not look like HTML")
	}
}

// "/try" is the URL the pages link to, and "/try.html" is the file that actually
// exists. Both have to answer with the same document, because the second one is
// what any link written before the first existed still points at.
func TestServeSite_ServesTheTryPageAtBothPaths(t *testing.T) {
	withoutDisk(t)

	atTry := get("/try")
	atHTML := get("/try.html")

	for _, tc := range []struct {
		path string
		rec  *httptest.ResponseRecorder
	}{{"/try", atTry}, {"/try.html", atHTML}} {
		if tc.rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", tc.path, tc.rec.Code)
		}
	}

	if atTry.Body.String() != atHTML.Body.String() {
		t.Error("/try and /try.html served different documents")
	}
	// And neither is the landing page: a mapping that collapsed both onto
	// index.html would satisfy everything above.
	if atTry.Body.String() == get("/").Body.String() {
		t.Error("/try served the landing page")
	}
}

// The placeholder is the one file in site/dist that is committed, so it is the
// only asset guaranteed to be in the binary wherever this test runs — which
// makes it the way to ask whether the embed carries dist/ at all.
//
// The dashboard serves its placeholder. The site refuses it, and this asserts
// that ServeSite's own answer is a 404 rather than "whatever is in the build
// directory" — the site serves what it claims, not what it happens to contain.
//
// Note what this does not claim: through the router the request never reaches
// ServeSite at all, because Claims rejects it before the site is consulted, so it
// is proxied like any other path the site does not own. That is the intended
// behaviour and it is tested in internal/server. This is only the second line of
// defence.
func TestEmbedCarriesDistButTheSiteDoesNotServeIt(t *testing.T) {
	withoutDisk(t)

	if !inEmbed("PLACEHOLDER") {
		t.Fatal("dist/PLACEHOLDER is not in the embed — //go:embed carried no dist/ at all")
	}

	rec := get("/PLACEHOLDER")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: the site must serve only what it claims", rec.Code)
	}
}

// The failure this project has already shipped once, in the other direction:
// every asset answering 200 with the page's own HTML, because a host with an SPA
// fallback cannot tell a missing file from a route. The site needs no fallback —
// both its pages are real build entries with real paths — so a missing asset is a
// 404, and the page appearing here instead would be that bug returning.
func TestServeSite_UnknownAssetIs404AndNotThePage(t *testing.T) {
	withoutDisk(t)

	page := get("/").Body.String()
	for _, target := range []string{"/assets/missing.css", "/assets/main-nope.js", "/favicon-missing.svg"} {
		rec := get(target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, rec.Code)
		}
		if rec.Body.String() == page {
			t.Errorf("GET %s served the page instead of a 404", target)
		}
	}
}

// ServeSite is only reached for paths Claims accepts, so this is defence in
// depth rather than a live path — but the cost of being wrong is a page served
// where a 404 belongs, which is the failure above.
func TestServeSite_UnclaimedPathIs404(t *testing.T) {
	withoutDisk(t)

	for _, target := range []string{"/pricing", "/_driftwood/api/traffic", "/try/extra", "/assetsfoo"} {
		if Claims(target) {
			t.Fatalf("%s is claimed; this test needs a path that is not", target)
		}
		if rec := get(target); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, rec.Code)
		}
	}
}

// A directory is not a file. Serving one would either list it or fail obscurely.
func TestServeSite_DoesNotServeDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	dd := distDir
	distDir = dir
	t.Cleanup(func() { distDir = dd })

	if rec := get("/assets"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /assets: status = %d, want 404", rec.Code)
	}
}

// A zero ModTime makes ServeContent omit Last-Modified, which leaves the browser
// to guess a cache lifetime. That is the cost of serving from the binary, and it
// is why the disk is checked first — this pins the cost so it stays deliberate.
//
// serveAsset is called directly rather than through ServeSite because it needs a
// file that is certainly in the embed, and the only such file is the placeholder,
// which the site deliberately does not claim.
func TestServeAsset_EmbeddedResponsesHaveNoLastModified(t *testing.T) {
	withoutDisk(t)

	rec := httptest.NewRecorder()
	if !serveAsset(rec, httptest.NewRequest(http.MethodGet, "/PLACEHOLDER", nil), "/PLACEHOLDER") {
		t.Fatal("serveAsset found nothing in the embed")
	}
	if lm := rec.Header().Get("Last-Modified"); lm != "" {
		t.Errorf("Last-Modified = %q; the embed reports a zero modtime and must not claim one", lm)
	}
}

func TestServeSite_PrefersDiskSoLastModifiedSurvives(t *testing.T) {
	dir := t.TempDir()
	const body = "console.log('from disk')"
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	dd := distDir
	distDir = dir
	t.Cleanup(func() { distDir = dd })

	rec := get("/assets/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != body {
		t.Errorf("body = %q, want %q", rec.Body.String(), body)
	}
	if rec.Header().Get("Last-Modified") == "" {
		t.Error("no Last-Modified: the disk copy lost to the embed, which is the caching regression this order exists to prevent")
	}
}

// The path reaches a filesystem lookup twice — once joined onto distDir and once
// as an embed.FS key — so anything that could walk out of either has to be
// refused before it gets there. fs.ValidPath is that refusal, and it is the only
// one; a hand-rolled ".." check would miss the encoded and normalised forms
// below.
//
// The decoy is what makes this test mean anything. Pointing distDir at an absent
// directory would leave every escaping path resolving to nothing whether or not
// the guard were there, so the test would pass against a version with no guard at
// all. Writing a real file one level above distDir means an escaped join finds it
// and serves it.
//
// Every target below is under /assets, which is the namespace Claims accepts —
// so each one gets past the claim check and is refused by the traversal guard,
// which is the guard under test. A target like "/../secret.txt" is turned away
// earlier and would prove nothing about it.
func TestServeSite_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	const secret = "SECRET-CONTENTS-MUST-NOT-BE-SERVED"
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}

	dd := distDir
	distDir = filepath.Join(root, "dist")
	t.Cleanup(func() { distDir = dd })

	for _, target := range []string{
		"/assets/../../secret.txt",
		"/assets/..%2f..%2fsecret.txt",
		"/assets/%2e%2e/%2e%2e/secret.txt",
		"/assets/./../../secret.txt",
		"/assets/....//../secret.txt",
		"/assets//../../secret.txt",
	} {
		rec := get(target)
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("GET %s served a file from outside distDir", target)
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, rec.Code)
		}
	}
}

func TestClaims(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		// Served.
		{"/", true},
		{"/index.html", true},
		{"/try", true},
		{"/try.html", true},
		{"/favicon.svg", true},
		{"/dashboard-light.png", true},
		{"/dashboard-dark.png", true},
		// The hashed-asset namespace, claimed whole because the file names change
		// on every build.
		{"/assets", true},
		{"/assets/main-HynccKc9.js", true},
		{"/assets/styles-BM_bvakk.css", true},
		// Not served — these belong to the proxied target, and the last three are
		// the ones a prefix comparison done carelessly would swallow.
		{"/pricing", false},
		{"/assetsfoo", false},
		{"/assets-old/main.js", false},
		{"/try/extra", false},
		{"/index.html/extra", false},
		// The control plane is out of reach by construction — internal/server asks
		// inside its !IsControlPath branch — but Claims must not claim it either,
		// so that the two defences agree.
		{"/_driftwood", false},
		{"/_driftwood/", false},
		{"/_driftwood/api/traffic", false},
		{"/_driftwood/mock/users", false},
	} {
		if got := Claims(tc.path); got != tc.want {
			t.Errorf("Claims(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// The committed placeholder keeps //go:embed compiling on a checkout that has
// never run the site build, which is exactly why its presence cannot be mistaken
// for a build: a dist holding only the placeholder has no pages to serve, and
// AssetsBuilt exists so cmd/drift can say so at startup instead of leaving a 404
// at "/" to be diagnosed as a routing bug.
//
// This asserts the disk half, which is the half a test can move. The embed half
// is fixed at compile time, so in a checkout where the site has been built
// AssetsBuilt answers true regardless — see the positive case below.
func TestPagesOnDisk_DistinguishesABuildFromThePlaceholder(t *testing.T) {
	dir := t.TempDir()

	if pagesOnDisk(dir) {
		t.Error("an empty dist reported a built site")
	}

	if err := os.WriteFile(filepath.Join(dir, "PLACEHOLDER"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if pagesOnDisk(dir) {
		t.Error("the placeholder alone reported a built site")
	}

	// One page without the other is not a build either: the two are separate
	// Rollup entries, and a config change that drops one still exits 0 and still
	// writes index.html.
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pagesOnDisk(dir) {
		t.Error("index.html without try.html reported a built site")
	}

	if err := os.WriteFile(filepath.Join(dir, "try.html"), []byte("<html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !pagesOnDisk(dir) {
		t.Error("both pages present did not report a built site")
	}
}

// The positive case, and the one that catches a build step that never ran: this
// checkout's site/dist is built, so the marker has to agree.
func TestAssetsBuilt(t *testing.T) {
	if !AssetsBuilt() {
		t.Error("AssetsBuilt reported no build in a checkout where site/dist is built — either the build step did not run, or the marker no longer matches what Vite emits")
	}
}

// trimLeadingSlash exists because fs.ValidPath rejects a leading slash, so the
// URL's own slash has to go before the path can be validated at all.
func TestTrimLeadingSlash(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/a", "a"}, {"//a", "a"}, {"a", "a"}, {"/", ""}, {"", ""},
	} {
		if got := trimLeadingSlash(tc.in); got != tc.want {
			t.Errorf("trimLeadingSlash(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
