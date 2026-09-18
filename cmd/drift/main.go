package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
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
	"github.com/donaina/driftwood/site"
	"github.com/donaina/driftwood/web"
)

func main() {
	// Handle subcommands
	if len(os.Args) > 1 && os.Args[1] == "import" {
		handleImport(os.Args[2:])
		return
	}

	target := flag.String("target", "http://localhost:3000", "Target API URL to proxy")
	port := flag.String("port", "8787", "Driftwood Proxy & Web Server Port")
	host := flag.String("host", "127.0.0.1", "Interface to bind. Loopback by default: the dashboard is an unauthenticated control plane, so exposing it to the network is a decision to make on purpose")
	// Off by default, and the help text says why rather than leaving it to be
	// discovered: "/" belongs to the proxied target otherwise, and answering
	// somebody's application root with a marketing page is not a thing to do
	// unasked.
	serveSite := flag.Bool("site", false, "Also serve the marketing site at / and /try. Off by default: with it on, \"/\" is answered by the site and no longer reaches the proxied target")
	flag.Parse()

	// Which flags the user actually typed. A flag left at its default is not a
	// decision, so it must not outrank a setting the dashboard saved — otherwise
	// launching with the default target would silently discard the target the
	// user had configured.
	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })

	// The flag is for a person; the environment variable is for a container.
	// Every other setting here is flag-only, and this one would be too — except
	// that a container's start command cannot always carry an argument while its
	// environment always can, and this is the one setting that has to be turned
	// on in production. A flag that was actually typed still wins, so
	// `--site=false` turns it back off where the variable is set.
	siteOn := *serveSite
	if !given["site"] {
		siteOn = envTruthy("DRIFTWOOD_SITE")
	}

	log.Println("==================================================")
	log.Println("⚡ Driftwood - Real-Time API Contract Drift Sniffer")
	log.Println("==================================================")

	// Initialize components
	store := storage.NewStore(*target, *port)
	if err := store.ApplyRememberedConfig(given["target"], given["port"]); err != nil {
		log.Printf("[Driftwood] %v — continuing with the command-line defaults", err)
	}
	// Naming a target answers the setup wizard's own question — "where is your
	// API running?" — so a first run with `--target https://api.acme.com` must
	// not then be asked it. Only --target counts: --port says where Driftwood's
	// own dashboard listens, which is not a statement about the API, and nearly
	// every launch passes one.
	if given["target"] {
		store.SetConfigured(true)
	}
	// Everything below reads the effective config, not the flags: once the saved
	// settings are overlaid, the flags are only one of its inputs.
	cfg := store.GetConfig()

	hub := events.NewHub()
	mockCtrl := mock.NewMockController()

	// Private targets are allowed here because the operator named this one: the
	// API being sniffed is usually on localhost. A target that arrives over HTTP
	// instead is checked — see Proxy.SetTarget.
	prx, err := proxy.NewProxyAllowPrivate(cfg.TargetURL, store, hub, mockCtrl)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	// Nil unless it was asked for, which is what keeps "/" proxied by default.
	var siteHandler http.Handler
	if siteOn {
		siteHandler = http.HandlerFunc(site.ServeSite)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl, siteHandler)
	addr := net.JoinHostPort(*host, cfg.ProxyPort)

	log.Printf("[Driftwood] Web Dashboard & Proxy running on http://%s", addr)
	// Said out loud because the failure it prevents is silent: a dashboard bound
	// to loopback is simply unreachable from another machine, and nothing else
	// in the output would explain why.
	if ip := net.ParseIP(*host); ip != nil && ip.IsLoopback() {
		log.Printf("[Driftwood] Bound to loopback only. Pass --host 0.0.0.0 to serve the network; the control plane has no authentication of its own.")
	}
	log.Printf("[Driftwood] Intercepting & forwarding traffic to %s", cfg.TargetURL)
	if siteOn {
		// Said out loud because it changes where "/" goes, which is otherwise
		// indistinguishable from a broken proxy.
		log.Printf("[Driftwood] Serving the marketing site at http://%s/ and http://%s/try. \"/\" is answered by the site and no longer reaches the target.", addr, addr)
	}
	log.Printf("[Driftwood] Built-in Mock Simulator: http://localhost:%s/_driftwood/mock/users", cfg.ProxyPort)
	// Said out loud for the same reason as the loopback note above: the failure
	// is otherwise silent and looks like a bug in the dashboard rather than a
	// missing build step. Every asset answers 200 with the page's own HTML, so
	// the dashboard renders unstyled and inert with nothing in the log to say
	// why. The proxy still sniffs and records correctly — this is the view, not
	// the engine — so it warns rather than refusing to start.
	if !web.AssetsBuilt() {
		log.Printf("[Driftwood] The dashboard has no built assets: web/dist holds only the placeholder that keeps //go:embed compiling. Run `make build` (or `npm --prefix frontend-react run build`). Until then every asset URL returns the page itself, so the dashboard will render with no styling and no scripts.")
	}
	// The same warning for the same reason. The site's failure mode is louder —
	// a missing build 404s rather than serving an unstyled page — but it is still
	// silent about why, and a 404 at "/" on a box where the binary started
	// cleanly reads as a routing bug rather than a missing build step.
	if siteOn && !site.AssetsBuilt() {
		log.Printf("[Driftwood] The site has no built assets: site/dist holds only the placeholder that keeps //go:embed compiling. Run `make build` (or `npm --prefix site run build`). Until then / and /try answer 404.")
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
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

// envTruthy reports whether name holds a value that means "on".
//
// Only the spellings people actually write are accepted, and anything else —
// including a typo, and including "false" — is off. An environment variable that
// turns a feature on has to fail in the direction that changes nothing.
func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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
