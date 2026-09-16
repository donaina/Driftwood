package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/openapi"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/server"
	"github.com/donaina/driftwood/internal/storage"
)

func main() {
	// Handle subcommands
	if len(os.Args) > 1 && os.Args[1] == "import" {
		handleImport(os.Args[2:])
		return
	}

	target := flag.String("target", "http://localhost:3000", "Target API URL to proxy")
	port := flag.String("port", "8787", "Driftwood Proxy & Web Server Port")
	flag.Parse()

	// Which flags the user actually typed. A flag left at its default is not a
	// decision, so it must not outrank a setting the dashboard saved — otherwise
	// launching with the default target would silently discard the target the
	// user had configured.
	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })

	log.Println("==================================================")
	log.Println("⚡ Driftwood - Real-Time API Contract Drift Sniffer")
	log.Println("==================================================")

	// Initialize components
	store := storage.NewStore(*target, *port)
	if err := store.ApplyRememberedConfig(given["target"], given["port"]); err != nil {
		log.Printf("[Driftwood] %v — continuing with the command-line defaults", err)
	}
	// Everything below reads the effective config, not the flags: once the saved
	// settings are overlaid, the flags are only one of its inputs.
	cfg := store.GetConfig()

	hub := events.NewHub()
	mockCtrl := mock.NewMockController()
	// Use NewProxyForTest to allow private IPs (like 127.0.0.1) for local testing and VPS deployment

	prx, err := proxy.NewProxyForTest(cfg.TargetURL, store, hub, mockCtrl)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl)
	// Bind to 0.0.0.0 (all interfaces) for external access
	addr := "0.0.0.0:" + cfg.ProxyPort

	log.Printf("[Driftwood] Web Dashboard & Proxy running on http://localhost:%s", cfg.ProxyPort)
	log.Printf("[Driftwood] Intercepting & forwarding traffic to %s", cfg.TargetURL)
	log.Printf("[Driftwood] Built-in Mock Simulator: http://localhost:%s/_driftwood/mock/users", cfg.ProxyPort)

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

	seedDemoBaselines(store)

	<-serverCtx.Done()
	fmt.Println("Driftwood server stopped.")
}

func handleImport(args []string) {
	importFlag := flag.NewFlagSet("import", flag.ExitOnError)
	specPath := importFlag.String("spec", "", "Path or URL to OpenAPI spec (required)")
	help := importFlag.Bool("help", false, "Show help")
	importFlag.Parse(args)

	if *help || *specPath == "" {
		fmt.Println("Usage: drift import -spec <file|url>")
		fmt.Println("  -spec    Path or URL to OpenAPI 3.x specification (required)")
		fmt.Println("  -help    Show this help")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  drift import -spec ./openapi.json")
		fmt.Println("  drift import -spec https://api.example.com/openapi.json")
		os.Exit(1)
	}

	// Load OpenAPI spec
	var spec *openapi.OpenAPISpec
	var err error

	if strings.HasPrefix(*specPath, "http://") || strings.HasPrefix(*specPath, "https://") {
		log.Printf("[Driftwood] Loading OpenAPI spec from URL: %s", *specPath)
		spec, err = openapi.LoadFromURL(*specPath)
	} else {
		log.Printf("[Driftwood] Loading OpenAPI spec from file: %s", *specPath)
		spec, err = openapi.LoadFromFile(*specPath)
	}
	if err != nil {
		log.Fatalf("Failed to load OpenAPI spec: %v", err)
	}

	log.Printf("[Driftwood] OpenAPI spec loaded: %s v%s", spec.Info.Title, spec.Info.Version)

	// Initialize storage with dummy target (we just need it for baseline storage)
	store := storage.NewStore("http://localhost:3000", "8787")

	// Import contracts
	err = spec.ImportToStorage(store)
	if err != nil {
		log.Fatalf("Failed to import contracts: %v", err)
	}

	log.Println("[Driftwood] OpenAPI contracts imported successfully!")
	contracts, _ := spec.ExtractContracts()
	for _, c := range contracts {
		log.Printf("  %s %s (operation: %s)", c.Method, c.Path, c.OperationID)
	}
}

// demoBaselinePath is the only endpoint Driftwood seeds a contract for: the
// mock simulator it serves itself.
const demoBaselinePath = "/_driftwood/mock/users"

const demoBaselinePayload = `{"id": 99812, "username": "alex_dev", "email": "alex@company.com", "score": 98.5, "is_active": true, "roles": ["admin", "developer"]}`

// seedDemoBaselines gives the built-in mock simulator a contract, so the demo
// has something to compare against out of the box.
//
// The one endpoint it may touch is the mock, which Driftwood serves itself —
// that is what makes seeding it a fixture rather than an invention. There used
// to be a second seed, for /api/users: a real path on the user's own API, given
// a contract fabricated here that no response had ever produced. One healthy
// GET /api/users returning 200 application/json then reported
// BREAKING / REMOVED_FIELD / $.username and raised an alert, because the real
// response did not contain a field Driftwood had invented. The first thing the
// product did to a new user's API was accuse it of breaking.
//
// Only seeds when no baseline exists, so a locked one is never overwritten.
func seedDemoBaselines(store *storage.Store) {
	if _, exists := store.GetBaseline("GET", demoBaselinePath); exists {
		return
	}
	_, _ = store.SaveBaseline("GET", demoBaselinePath, demoBaselinePayload)
}
