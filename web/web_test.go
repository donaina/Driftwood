package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withoutDisk points the on-disk lookup at directories that do not exist, so the
// embedded copy is the only source a test can be reading from.
//
// This is not cosmetic. The files in web/dist are build output and are not
// committed, so whether they are present depends on whether the machine running
// the test has run a Vite build — which would make these tests pass in one
// checkout and fail in another. The embed is compiled in, so it is the same
// everywhere.
func withoutDisk(t *testing.T) {
	t.Helper()
	wd, dd := webDir, distDir
	webDir, distDir = filepath.Join(t.TempDir(), "absent"), filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { webDir, distDir = wd, dd })
}

func get(target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	ServeIndex(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestServeIndex_ServesThePageFromTheEmbed(t *testing.T) {
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

// An unmatched path is the page, not a 404: the React views are mounted into one
// document, so a deep link has nothing else to resolve to.
func TestServeIndex_UnknownPathFallsBackToThePage(t *testing.T) {
	withoutDisk(t)

	page := get("/").Body.String()
	for _, p := range []string{"/some/deep/route", "/nope.js", "/assets/missing.css"} {
		if got := get(p).Body.String(); got != page {
			t.Errorf("%s did not fall back to the page (%d bytes vs %d)", p, len(got), len(page))
		}
	}
}

// The placeholder is the one file in web/dist that is committed, so it is the
// only asset guaranteed to be in the binary wherever this test runs. Asserting
// on it is what proves the embed carries dist/ at all.
func TestServeIndex_ServesEmbeddedAssets(t *testing.T) {
	withoutDisk(t)

	rec := get("/PLACEHOLDER")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — is dist/ missing from the embed?", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go:embed is a compile error") {
		t.Error("served something other than the embedded placeholder")
	}
}

// A zero ModTime makes ServeContent omit Last-Modified, which leaves the browser
// to guess a cache lifetime. That is the cost of serving from the binary, and it
// is why the disk is checked first — this pins the cost so it stays deliberate.
func TestServeIndex_EmbeddedResponsesHaveNoLastModified(t *testing.T) {
	withoutDisk(t)

	if lm := get("/PLACEHOLDER").Header().Get("Last-Modified"); lm != "" {
		t.Errorf("Last-Modified = %q; the embed reports a zero modtime and must not claim one", lm)
	}
}

func TestServeIndex_PrefersDiskSoLastModifiedSurvives(t *testing.T) {
	dir := t.TempDir()
	const body = "console.log('from disk')"
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	dd := distDir
	distDir = dir
	t.Cleanup(func() { distDir = dd })

	rec := get("/app.js")
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

// A directory is not a file. Serving one would either list it or fail obscurely;
// falling through to the page is the same answer as any other unmatched path.
func TestServeIndex_DoesNotServeDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	dd := distDir
	distDir = dir
	t.Cleanup(func() { distDir = dd })

	if got := get("/assets").Body.String(); got != get("/").Body.String() {
		t.Error("a directory request did not fall back to the page")
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
// the guard were there, so the test would pass against a version with no guard
// at all. Writing a real file one level above distDir means an escaped join
// finds it and serves it.
func TestServeIndex_RejectsTraversal(t *testing.T) {
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

	page := get("/").Body.String()
	for _, target := range []string{
		"/../secret.txt",
		"/..%2fsecret.txt",
		"/%2e%2e/secret.txt",
		"/assets/../../secret.txt",
		"/./../secret.txt",
		"/....//secret.txt",
	} {
		body := get(target).Body.String()
		if strings.Contains(body, secret) {
			t.Errorf("%s served a file from outside distDir", target)
		}
		if body != page {
			t.Errorf("%s did not fall back to the page", target)
		}
	}
}

// trimLeadingSlash exists because fs.ValidPath rejects a leading slash, so the
// URL's own slash has to go before the path can be validated at all.
// The shell has to declare its own icon, and the href has to be relative.
//
// Both halves are about one request. With no icon in the document a browser
// asks for /favicon.ico at the ORIGIN root, and the origin root is the proxy —
// so every dashboard page load reaches the target as a request nobody made,
// and Driftwood records it. It appears in Live Network Traffic under a name the
// API does not serve, it counts toward the error rate when the API answers 404,
// and when the answer looks like JSON it is baselined and can raise a contract
// alert against a phantom endpoint. The dashboard would be writing traffic into
// the log whose whole claim is that it holds requests Driftwood sniffed.
//
// A root-absolute href is not a fix: /favicon.svg is a path on the API, so it
// asks the user's server for Driftwood's icon — the same phantom under a new
// name. Everything the shell loads is relative for the reason the StripPrefix
// comment in internal/server/server.go gives: the page is served from under
// /_driftwood/, and every other path belongs to the API.
//
// The file the href names is asserted by the `dashboard` target in the Makefile
// rather than here, because whether the build produced it is not a question
// this package can answer without one.
func TestShellDeclaresItsOwnIcon(t *testing.T) {
	withoutDisk(t)

	page := get("/").Body.String()
	const marker = `<link rel="icon"`
	i := strings.Index(page, marker)
	if i < 0 {
		t.Fatal("the shell declares no icon, so a browser will ask the API's root for /favicon.ico and the proxy will record it as traffic")
	}
	end := strings.Index(page[i:], ">")
	if end < 0 {
		t.Fatalf("unterminated link tag at offset %d", i)
	}

	tag := page[i : i+end+1]
	href := ""
	if _, rest, ok := strings.Cut(tag, `href="`); ok {
		href, _, _ = strings.Cut(rest, `"`)
	}
	if href == "" {
		t.Fatalf("the icon link has no href: %s", tag)
	}
	if strings.HasPrefix(href, "/") {
		t.Errorf("icon href = %q is root-absolute, so it names a path on the API rather than on Driftwood", href)
	}
}

func TestTrimLeadingSlash(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/a", "a"}, {"//a", "a"}, {"a", "a"}, {"/", ""}, {"", ""},
	} {
		if got := trimLeadingSlash(tc.in); got != tc.want {
			t.Errorf("trimLeadingSlash(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The committed placeholder keeps //go:embed compiling on a checkout that has
// never run the frontend build, which is exactly why its presence cannot be
// mistaken for a build: a dist holding only the placeholder serves the page in
// place of every asset, and AssetsBuilt exists so a caller can tell.
func TestHasStylesheet(t *testing.T) {
	dir := t.TempDir()

	if hasStylesheet(dir) {
		t.Error("an empty dist reported built assets")
	}

	if err := os.WriteFile(filepath.Join(dir, "PLACEHOLDER"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if hasStylesheet(dir) {
		t.Error("the placeholder alone reported built assets")
	}

	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "driftwood.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasStylesheet(dir) {
		t.Error("a built stylesheet was not reported")
	}
}
