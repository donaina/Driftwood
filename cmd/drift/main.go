package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/server"
	"github.com/donaina/driftwood/internal/storage"
)

func main() {
	target := flag.String("target", "http://localhost:3000", "Target API URL to proxy")
	port := flag.String("port", "8787", "Driftwood Proxy & Web Server Port")
	flag.Parse()

	log.Println("==================================================")
	log.Println("⚡ Driftwood - Real-Time API Contract Drift Sniffer")
	log.Println("==================================================")

	// Initialize components
	store := storage.NewStore(*target, *port)
	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	prx, err := proxy.NewProxy(*target, store, hub, mockCtrl)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl)
	// Bind to 127.0.0.1 only (not all interfaces) for security
	addr := "127.0.0.1:" + *port

	log.Printf("[Driftwood] Web Dashboard & Proxy running on http://localhost:%s", *port)
	log.Printf("[Driftwood] Intercepting & forwarding traffic to %s", *target)
	log.Printf("[Driftwood] Built-in Mock Simulator: http://localhost:%s/_driftwood/mock/users", *port)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	// Graceful shutdown context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down Driftwood server...")

		// Shutdown proxy first
		shutdownCtx, cancel := context.WithTimeout(serverCtx, 10*time.Second)
		defer cancel()
		if err := prx.Shutdown(shutdownCtx); err != nil {
			log.Printf("Proxy shutdown error: %v", err)
		}

		// Shutdown HTTP server
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		serverStopCtx()
	}()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Seed initial baseline for demo simulator so user has instant out-of-the-box baseline contract
	// Only seed if baselines don't already exist (don't overwrite user-locked ones)
	if _, exists := store.GetBaseline("GET", "/_driftwood/mock/users"); !exists {
		_, _ = store.SaveBaseline("GET", "/_driftwood/mock/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)
	}
	if _, exists := store.GetBaseline("GET", "/api/users"); !exists {
		_, _ = store.SaveBaseline("GET", "/api/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)
	}

	<-serverCtx.Done()
	fmt.Println("Driftwood server stopped.")
}