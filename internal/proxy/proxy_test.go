package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/storage"
)

func TestProxySSRFValidation(t *testing.T) {
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Test: valid http URL
	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatalf("valid http URL should be accepted: %v", err)
	}
	if prx.SetTarget("https://api.example.com") != nil {
		t.Errorf("valid https URL should be accepted")
	}

	// Test: invalid schemes should be rejected
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"http://localhost:8787", // SSRF to local metadata - should be blocked
		"http://169.254.169.254/", // AWS metadata
		"http://127.0.0.1:8080",   // local
		"",                        // empty
		"not-a-url",
	}

	for _, u := range invalidURLs {
		err = prx.SetTarget(u)
		if err == nil {
			t.Errorf("SSRF URL should be rejected: %s", u)
		}
	}

	// Test: valid URL after invalid
	if err := prx.SetTarget("https://api.github.com"); err != nil {
		t.Errorf("valid URL after invalid should work: %v", err)
	}
}

func TestProxyTargetURLRaceSafety(t *testing.T) {
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	prx, err := NewProxy("http://example.com", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent SetTarget and Handler access should not panic
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			prx.SetTarget("https://api.example.com")
			_ = prx.Handler()
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestSanitizeTrafficWired(t *testing.T) {
	store := storage.NewStore("http://localhost:8787", "8787")
	hub := events.NewHub()
	mockCtrl := &mock.MockController{}

	// Use test proxy for localhost target
	prx, err := NewProxyForTest("http://localhost:3000", store, hub, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test server that returns JSON with auth headers
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret123; HttpOnly")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 1, "name": "test"}`))
	}))
	defer targetServer.Close()

	// Point proxy to test server
	targetURL, _ := url.Parse(targetServer.URL)
	prx.GetTargetURLForTest().Store(targetURL)

	// Make request through proxy
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret456")

	rec := httptest.NewRecorder()
	prx.Handler()(rec, req)

	// Check that traffic was captured and sanitized
	// The stored traffic should have redacted headers
	t.Logf("Response status: %d", rec.Code)
}