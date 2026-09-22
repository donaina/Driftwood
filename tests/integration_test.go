package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/server"
	"github.com/donaina/driftwood/internal/storage"
)

func TestEndToEndProxyAndDiff(t *testing.T) {
	/* storage.NewStore resolves its persist path from the home directory, and
	   the first JSON response through the proxy is auto-saved as a baseline —
	   so without this the suite writes test endpoints into the developer's real
	   ~/.driftwood/baselines.json. GET:/api/products is one of them, and it is
	   this test that put it there. internal/server and cmd/drift already do
	   this; this file and internal/proxy did not. */
	t.Setenv("HOME", t.TempDir())

	// Start mock backend server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 100, "name": "Product A", "active": true}`))
	}))
	defer targetServer.Close()

	// Initialize Driftwood components
	store, err := storage.NewStore(targetServer.URL, "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	// Use test proxy that allows private IPs
	prx, err := proxy.NewProxyAllowPrivate(targetServer.URL, store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl, nil, nil)
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
	baseline, exists := store.GetBaseline(store.ActiveProject(), "GET", "/api/products")
	if !exists {
		t.Fatalf("expected baseline contract to be created")
	}

	if baseline.Schema == nil {
		t.Fatalf("expected schema to be non-nil")
	}
}
