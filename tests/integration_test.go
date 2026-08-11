package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/callmidavid/apidiff/internal/events"
	"github.com/callmidavid/apidiff/internal/mock"
	"github.com/callmidavid/apidiff/internal/proxy"
	"github.com/callmidavid/apidiff/internal/server"
	"github.com/callmidavid/apidiff/internal/storage"
)

func TestEndToEndProxyAndDiff(t *testing.T) {
	// Start mock backend server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 100, "name": "Product A", "active": true}`))
	}))
	defer targetServer.Close()

	// Initialize APIDiff components
	store := storage.NewStore(targetServer.URL, "8787")
	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	prx, err := proxy.NewProxy(targetServer.URL, store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl)
	testProxyServer := httptest.NewServer(srv.Router())
	defer testProxyServer.Close()

	// 1. Initial request establishes baseline
	resp, err := http.Get(testProxyServer.URL + "/api/products")
	if err != nil {
		t.Fatalf("failed to call proxied server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify baseline was created
	baseline, exists := store.GetBaseline("GET", "/api/products")
	if !exists {
		t.Fatalf("expected baseline contract to be created")
	}

	if baseline.Schema == nil {
		t.Fatalf("expected schema to be non-nil")
	}
}
