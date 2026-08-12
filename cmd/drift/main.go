package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	addr := ":" + *port

	log.Printf("[Driftwood] Web Dashboard & Proxy running on http://localhost:%s", *port)
	log.Printf("[Driftwood] Intercepting & forwarding traffic to %s", *target)
	log.Printf("[Driftwood] Built-in Mock Simulator: http://localhost:%s/_driftwood/mock/users", *port)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Seed initial baseline for demo simulator so user has instant out-of-the-box baseline contract
	_, _ = store.SaveBaseline("GET", "/_driftwood/mock/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)
	_, _ = store.SaveBaseline("GET", "/api/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)

	// Graceful shutdown listener
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("\nShutting down Driftwood server...")
}
