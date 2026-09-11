package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	// If requesting a static asset from the React build, serve it directly
	if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.svg" || r.URL.Path == "/icons.svg" {
		// Base directory for the React app build
		baseDir := filepath.Join("frontend-react", "dist")
		// Serve from dist directory
		fullPath := filepath.Join(baseDir, r.URL.Path[1:]) // remove leading slash
		if _, err := os.Stat(fullPath); err == nil {
			http.ServeFile(w, r, fullPath)
			return
		}
	}

	// Serve the main HTML file (web/index.html)
	indexPath := filepath.Join("web", "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, indexPath)
}