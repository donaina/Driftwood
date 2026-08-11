package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/callmidavid/apidiff/internal/events"
	"github.com/callmidavid/apidiff/internal/mock"
	"github.com/callmidavid/apidiff/internal/proxy"
	"github.com/callmidavid/apidiff/internal/server"
	"github.com/callmidavid/apidiff/internal/storage"
)

func main() {
	target := flag.String("target", "http://localhost:3000", "Target API URL to proxy")
	port := flag.String("port", "8787", "APIDiff Proxy & Web Server Port")
	flag.Parse()

	log.Println("==================================================")
	log.Println("⚡ APIDiff - Real-Time API Contract Drift Sniffer")
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

	log.Printf("[APIDiff] Web Dashboard & Proxy running on http://localhost:%s", *port)
	log.Printf("[APIDiff] Intercepting & forwarding traffic to %s", *target)
	log.Printf("[APIDiff] Built-in Mock Simulator: http://localhost:%s/_apidiff/mock/users", *port)

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
	_, _ = store.SaveBaseline("GET", "/_apidiff/mock/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)
	_, _ = store.SaveBaseline("GET", "/api/users", `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`)

	// Graceful shutdown listener
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("\nShutting down APIDiff server...")
}
