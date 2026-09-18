// Package site serves the marketing site from inside the binary.
//
// The site is a landing page and a try-it-out page, and it is served from here
// rather than from a static host because of what the try page does: it drives the
// live control API same-origin. Splitting the two across origins would mean
// widening internal/server's CORS allowlist, which is deliberately a hardcoded
// localhost list, purely to serve static files. One origin needs no CORS change
// at all, ships as one artifact at one commit behind one rollback, and makes the
// README's "single binary, move it anywhere on its own" claim true of the whole
// product rather than half of it.
//
// The output is site/dist rather than a dist/ of the site package's own making
// because //go:embed cannot reach outside its own package directory — the same
// wall web/web.go ran into. Putting the build here lets one package own the whole
// site, the pages and the bundles they load.
//
// This package is only ever reached when the operator asked for it. The site
// claims "/", and "/" is otherwise the proxied target, so it is opt-in: see the
// --site flag in cmd/drift.
package site

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:dist
var embedded embed.FS

// distDir is the on-disk location, relative to the process's working directory. A
// file found here wins over the embedded copy, so editing a bundle is visible
// without rebuilding the binary.
var distDir = filepath.Join("site", "dist")

// assetsPrefix is Vite's output directory for everything content-hashed. It is
// claimed as a namespace rather than as a file list because the file names change
// on every build.
const assetsPrefix = "/assets"

// claimed maps a request path to the file that answers it. One map rather than a
// predicate plus a lookup, so the two can never disagree about what the site owns
// — a path that is claimed but unmapped would be a path that answers 500.
//
// Every entry is an exact path. Nothing here is a prefix, so a deeper path under
// one of them is unclaimed and belongs to the proxied target, exactly as any
// other path outside this map does.
var claimed = map[string]string{
	"/":                    "index.html",
	"/index.html":          "index.html",
	"/try":                 "try.html",
	"/try.html":            "try.html",
	"/favicon.svg":         "favicon.svg",
	"/dashboard-light.png": "dashboard-light.png",
	"/dashboard-dark.png":  "dashboard-dark.png",
}

// pages are the documents a real build always emits, at names it does not hash.
// They are the marker AssetsBuilt looks for; see the comment there for why the
// stylesheet — the dashboard's marker — cannot serve.
var pages = []string{"index.html", "try.html"}

// Claims reports whether the site owns path.
//
// The caller must consult this only for paths outside the control namespace. That
// is not a convention to remember: internal/server asks inside its
// !IsControlPath branch, so the site is structurally incapable of shadowing
// /_driftwood/*.
func Claims(path string) bool {
	if _, ok := claimed[path]; ok {
		return true
	}
	return inNamespace(path, assetsPrefix)
}

// inNamespace reports whether path is prefix itself or sits beneath it, so
// "/assetsfoo" is not swallowed by the "/assets" namespace. Mirrors
// proxy.inNamespace, which is unexported; it is four lines and duplicating it
// costs less than the dependency.
func inNamespace(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// ServeSite answers a request the site claims.
//
// A claimed path with no file behind it is a 404. It is never the page and never
// the proxy, and both halves of that matter. Answering with index.html is how a
// static host's SPA fallback makes every unknown asset return 200 and the page's
// own HTML, which is a failure this project has already shipped once and has
// written down in site/src/lib/driftwood.ts. Falling through to the proxy would
// hand a mistyped asset URL to a backend that has no idea what it is, and a
// backend that catches all routes would answer 200 with something that looks
// plausible — the same failure wearing a different hat.
func ServeSite(w http.ResponseWriter, r *http.Request) {
	name, ok := claimed[r.URL.Path]
	if !ok {
		if !inNamespace(r.URL.Path, assetsPrefix) {
			http.NotFound(w, r)
			return
		}
		name = r.URL.Path
	}
	if !serveAsset(w, r, name) {
		http.NotFound(w, r)
	}
}

// serveAsset writes the build file the request names, and reports whether it
// found one.
func serveAsset(w http.ResponseWriter, r *http.Request, name string) bool {
	name = trimLeadingSlash(name)
	// ValidPath rejects "..", absolute paths and empty segments, so neither
	// lookup below can be walked out of the directory it serves from. Do not
	// replace this with a manual ".." check: it is the whole traversal defence,
	// and it is the reason a URL-encoded or normalised escape never reaches a
	// filesystem call.
	if name == "" || !fs.ValidPath(name) {
		return false
	}

	// On disk first. embed.FS reports a zero ModTime, so serving from it makes
	// ServeContent omit Last-Modified and leaves the browser to guess a cache
	// lifetime from the response's age — which is how a rebuilt bundle goes stale
	// between deploys. ServeFile emits the real header for a file that is there.
	if p := filepath.Join(distDir, filepath.FromSlash(name)); isFile(p) {
		http.ServeFile(w, r, p)
		return true
	}

	if data, err := fs.ReadFile(embedded, "dist/"+name); err == nil {
		// The name is passed so ServeContent can infer the Content-Type from the
		// extension; the zero modtime means no Last-Modified, which is the
		// documented cost of serving from the binary.
		http.ServeContent(w, r, filepath.Base(name), time.Time{}, bytes.NewReader(data))
		return true
	}
	return false
}

// AssetsBuilt reports whether the site was actually built.
//
// site/dist/PLACEHOLDER is committed deliberately, for the reason web's is:
// //go:embed is a compile error when the directory it names is absent, so without
// the placeholder a clean checkout cannot `go build ./...` at all. The price of
// keeping the build working is that it succeeds whether or not anyone ran the
// site step, and the binary then serves two pages whose stylesheet and scripts it
// does not have — every asset URL answering 404 rather than the page, which is
// at least honest but still looks like a broken deploy and says nothing in the
// log. A caller that wants to know has to ask.
//
// The marker is the pages, not the stylesheet. The dashboard can name its own
// because the dashboard's build emits it at a fixed path; Vite content-hashes the
// site's CSS, so that name changes on every build and cannot be written down
// here. Both HTML files keep their names, and a build that emitted one without
// the other did not succeed.
func AssetsBuilt() bool {
	if pagesOnDisk(distDir) {
		return true
	}
	for _, page := range pages {
		if !inEmbed(page) {
			return false
		}
	}
	return true
}

// pagesOnDisk reports whether dir holds every document a real build emits.
//
// Takes its directory as an argument, and is separate from AssetsBuilt, so the
// marker logic is testable. AssetsBuilt is not: it falls back to the embed, which
// is fixed at compile time, so in any checkout where the site has been built it
// answers true no matter what a test does to a temporary directory. This is the
// half that has to tell a real build from a dist holding only the placeholder.
func pagesOnDisk(dir string) bool {
	for _, page := range pages {
		if !isFile(filepath.Join(dir, filepath.FromSlash(page))) {
			return false
		}
	}
	return true
}

// inEmbed reports whether the binary carries dist/name.
func inEmbed(name string) bool {
	_, err := fs.Stat(embedded, "dist/"+name)
	return err == nil
}

func trimLeadingSlash(p string) string {
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	return p
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
