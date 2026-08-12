package web

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
