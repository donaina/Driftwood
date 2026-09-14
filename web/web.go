package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"fmt"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	// DEBUG: Log the request path
	fmt.Printf("DEBUG: Request path: %s\n", r.URL.Path)
	// If requesting a static asset from the React build, serve it directly
	if strings.HasPrefix(r.URL.Path, "/assets/") || 
	   strings.HasSuffix(r.URL.Path, ".js") || 
	   strings.HasSuffix(r.URL.Path, ".svg") {
	   	fmt.Printf("DEBUG: Matched asset condition for path: %s\n", r.URL.Path)
		// Base directory for the React app build
		baseDir := filepath.Join("frontend-react", "dist")
		// Serve from dist directory
		fullPath := filepath.Join(baseDir, r.URL.Path[1:]) // remove leading slash
		fmt.Printf("DEBUG: Looking for file at: %s\n", fullPath)
		if _, err := os.Stat(fullPath); err == nil {
			fmt.Printf("DEBUG: Found file, serving: %s\n", fullPath)
			http.ServeFile(w, r, fullPath)
			return
		} else {
			fmt.Printf("DEBUG: File not found: %v\n", err)
		}
	}

	fmt.Printf("DEBUG: Serving index.html for path: %s\n", r.URL.Path)
	// Serve the main HTML file (web/index.html)
	indexPath := filepath.Join("web", "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, indexPath)
}
