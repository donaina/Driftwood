package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The dashboard ships inside the binary.
//
// It did not before: web.ServeIndex read both the page and its bundles off the
// filesystem, relative to the process's working directory. A binary started
// anywhere else served a dashboard with no stylesheet and no scripts, which is
// why the systemd unit pins a WorkingDirectory to a checkout, and why the npm
// package — which ships this Go source but no Vite output — installed a
// dashboard that could not load its own assets.
//
// The output is web/dist rather than frontend-react/dist because //go:embed
// cannot reach outside its own package directory, and frontend-react/ is a
// sibling of web/. Putting the build here lets one package own the whole
// dashboard, the page and the bundles it loads.
//
//go:embed index.html
//go:embed all:dist
var embedded embed.FS

// webDir and distDir are the on-disk locations, relative to the process's
// working directory. A file found here wins over the embedded copy, so editing
// a bundle is visible without rebuilding the binary.
var (
	webDir  = "web"
	distDir = filepath.Join("web", "dist")
)

// dashboardStylesheet is the marker of a real frontend build: every build emits
// it and nothing else does.
const dashboardStylesheet = "assets/driftwood.css"

// AssetsBuilt reports whether the frontend was actually built.
//
// web/dist/PLACEHOLDER is committed deliberately. //go:embed is a compile error
// when the directory it names is absent, so without the placeholder a clean
// checkout cannot `go build ./...` at all, and with it `make verify` cannot
// either. The price of keeping the build working is that it succeeds whether or
// not anyone ran the frontend step, and the binary then serves a dashboard whose
// stylesheet and every one of its scripts are index.html. Nothing in the
// response admits it — each asset URL answers 200, with HTML — so a caller that
// wants to know has to ask.
func AssetsBuilt() bool {
	if hasStylesheet(distDir) {
		return true
	}
	_, err := fs.Stat(embedded, "dist/"+dashboardStylesheet)
	return err == nil
}

// hasStylesheet reports whether dir holds the dashboard's stylesheet.
func hasStylesheet(dir string) bool {
	return isFile(filepath.Join(dir, filepath.FromSlash(dashboardStylesheet)))
}

// ServeIndex serves the dashboard: the build file a path names if there is one,
// and the page itself otherwise. The React views are mounted by scripts into a
// single page, so an unmatched path is the page, not a 404.
func ServeIndex(w http.ResponseWriter, r *http.Request) {
	if serveAsset(w, r) {
		return
	}
	servePage(w, r)
}

// serveAsset writes the build file the request names, and reports whether it
// found one.
func serveAsset(w http.ResponseWriter, r *http.Request) bool {
	name := trimLeadingSlash(r.URL.Path)
	// ValidPath rejects "..", absolute paths and empty segments, so neither
	// lookup below can be walked out of the directory it serves from. Do not
	// replace this with a manual ".." check: it is the whole traversal defence.
	if name == "" || !fs.ValidPath(name) {
		return false
	}

	// On disk first. embed.FS reports a zero ModTime, so serving from it makes
	// ServeContent omit Last-Modified and leaves the browser to guess a cache
	// lifetime from the response's age — which is how a rebuilt stylesheet goes
	// stale, the exact failure the stable unhashed filename exists to avoid.
	// ServeFile emits the real header for a file that is really there.
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

func servePage(w http.ResponseWriter, r *http.Request) {
	if p := filepath.Join(webDir, "index.html"); isFile(p) {
		http.ServeFile(w, r, p)
		return
	}

	data, err := embedded.ReadFile("index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
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
