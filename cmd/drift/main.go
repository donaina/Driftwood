package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/donaina/driftwood/internal/events"
	"github.com/donaina/driftwood/internal/mock"
	"github.com/donaina/driftwood/internal/openapi"
	"github.com/donaina/driftwood/internal/proxy"
	"github.com/donaina/driftwood/internal/server"
	"github.com/donaina/driftwood/internal/storage"
	"github.com/donaina/driftwood/internal/webhook"
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
	//
	// A store that could not be fully read still starts: the error says what
	// happened to the saved contracts, and refusing to run over it would turn a
	// recoverable file problem into an outage. Not saying anything at all is the
	// behaviour this replaced, and it is the worse of the two — an install that
	// silently comes up empty is indistinguishable from a new one.
	store, err := storage.NewStore(*target, *port)
	if err != nil {
		log.Printf("[Driftwood] %v", err)
	}
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

	// The deliverer is built before the proxy because the proxy is handed it as
	// an option, and started after: Run spawns the workers, and nothing should be
	// able to enqueue into a deliverer that has not started.
	//
	// It is not conditional on any channel being configured. Config is editable
	// while the process runs — that is what the webhooks API is for — so a
	// deliverer that only existed when a URL was already saved would need
	// starting from the route that saves the first one.
	deliverer := webhook.New(store, hub)

	// Private targets are allowed here because the operator named this one: the
	// API being sniffed is usually on localhost. A target that arrives over HTTP
	// instead is checked — see Proxy.SetTarget.
	prx, err := buildProxy(cfg.TargetURL, store, hub, mockCtrl, proxy.WithDelivery(deliverer))
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}
	deliverer.Run()

	// Nil unless it was asked for, which is what keeps "/" proxied by default.
	var siteHandler http.Handler
	if siteOn {
		siteHandler = http.HandlerFunc(site.ServeSite)
	}

	srv := server.NewServer(store, hub, prx, mockCtrl, siteHandler, deliverer)
	addr := net.JoinHostPort(*host, cfg.ProxyPort)

	/* The port is taken here rather than inside the goroutine below, and the
	   difference is not stylistic: the hint file that tells `drift import` where
	   to find this instance must not be written by an instance that failed to
	   bind. Binding first makes the hint true by construction — it is written
	   only once this process actually holds the port — instead of true in the
	   ordinary case and misleading in the one where something else already had
	   it. There is still a window between the bind and the first request being
	   served, but nothing can be listening on a port without owning it, so the
	   probe that reads this hint answers correctly throughout. */
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	/* Recorded so the CLI can find a running instance and ask it to make the
	   import, rather than writing the store file underneath one — see
	   handleImport for what that used to cost. Best-effort: a failure here costs
	   the CLI its fast path and nothing else, and the CLI warns on the slow path
	   anyway, so it is a log line rather than a reason not to start. */
	controlHost := *host
	if ip := net.ParseIP(controlHost); ip == nil || !ip.IsLoopback() {
		// The CLI runs on this machine, so a wildcard or public bind is still
		// reached over loopback. Naming the bound address instead would hand the
		// CLI an address it may not be able to route to — and one that leaves the
		// machine, which is not what an import needs.
		controlHost = "127.0.0.1"
	}
	controlURL := "http://" + net.JoinHostPort(controlHost, cfg.ProxyPort)
	if err := storage.WriteInstance(storage.InstanceInfo{
		ControlURL: controlURL,
		PID:        os.Getpid(),
		StartedAt:  time.Now(),
	}); err != nil {
		log.Printf("[Driftwood] Could not record where this instance is listening (%v). A `drift import` run while this is up will not find it and will write the store file directly, which this instance will overwrite on its next save.", err)
	}

	log.Printf("[Driftwood] Web Dashboard & Proxy running on http://%s", addr)
	// Said out loud because the failure it prevents is silent: a dashboard bound
	// to loopback is simply unreachable from another machine, and nothing else
	// in the output would explain why.
	if ip := net.ParseIP(*host); ip != nil && ip.IsLoopback() {
		log.Printf("[Driftwood] Bound to loopback only. Pass --host 0.0.0.0 to serve the network; the control plane has no authentication of its own.")
	}
	if strings.TrimSpace(cfg.TargetURL) == "" {
		// Named rather than left as "forwarding traffic to ", which reads like a
		// broken target instead of an absent one, and said alongside where to fix
		// it because the dashboard is the only place that can.
		log.Printf("[Driftwood] The active project has no target, so nothing is being forwarded. Set one in Proxy Settings; every request answers 502 until then.")
	} else {
		log.Printf("[Driftwood] Intercepting & forwarding traffic to %s", cfg.TargetURL)
	}
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

		// The deliverer drains last, and on its own budget.
		//
		// Last because an alert raised by a request still being handled is a
		// delivery the operator is owed; once the listener is closed there are no
		// more, so this is the point where the queue is final. Its own budget
		// because shutdownCtx is shared and already partly spent — a delivery
		// still retrying should get a fresh window rather than whatever is left
		// of the server's drain, and a slow receiver should not be able to eat
		// the time the server needs to finish.
		drainCtx, cancelDeliveries := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelDeliveries()
		if err := deliverer.Close(drainCtx); err != nil {
			log.Printf("Alert deliveries: %v", err)
		}

		/* Removed last, and only if it still names this process.

		   Last because the hint is what sends an import to this instance rather
		   than to the file, and it must keep being true for as long as anything
		   here can serve it — a hint removed while the listener is still open
		   would route a concurrent `drift import` to the file behind a live
		   proxy, which is the bug this whole path exists to prevent.

		   Only if it names this process, because the listener closed several
		   steps ago and this one is still running: a replacement instance can
		   have bound the port and recorded itself in the meantime, and deleting
		   the file unconditionally would take away the address of the instance
		   that is actually serving. ClearInstance checks before it removes, so
		   what survives here is the other process's live hint rather than this
		   one's dead one. */
		storage.ClearInstance(os.Getpid())
		serverStopCtx()
	}()

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
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
// buildProxy assembles the proxy for the active project's target, and is
// separated from main because the bug it fixes lived in a log.Fatalf — which no
// test can get past, so the branch that crashed production was the branch
// nothing could cover.
//
// An empty target is not an error. It means the active project has no backend,
// which the dashboard creates on purpose: CreateProject accepts an empty
// target_url and documents the resulting project as allowed to be in that
// state, and the request path answers it with a 502. Treating it as fatal here
// was the outlier, and it made one client's missing backend into an outage for
// every other client on the install.
func buildProxy(target string, store *storage.Store, hub *events.Hub, mockCtrl *mock.MockController, opts ...proxy.Option) (*proxy.Proxy, error) {
	if strings.TrimSpace(target) == "" {
		return proxy.NewProxyWithoutTarget(store, hub, mockCtrl, opts...)
	}
	// Private targets are allowed here because the operator named this one: the
	// API being sniffed is usually on localhost. A target that arrives over HTTP
	// instead is checked — see SetProjectTarget.
	return proxy.NewProxyAllowPrivate(target, store, hub, mockCtrl, opts...)
}

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
	projectFlag := importFlag.String("project", "", "Project id to file the contracts under (default: the active project)")
	help := importFlag.Bool("help", false, "Show help")
	importFlag.Parse(args)

	if *help || *specPath == "" {
		fmt.Println("Usage: drift import -spec <file|url> [-project <id>]")
		fmt.Println("  -spec     Path or URL to OpenAPI 3.x specification (required)")
		fmt.Println("  -project  Project id to file the contracts under")
		fmt.Println("            (default: whichever project is active)")
		fmt.Println("  -help     Show this help")
		fmt.Println()
		fmt.Println("If a Driftwood instance is running, the contracts are imported through it.")
		fmt.Println("Otherwise the store file is written directly, and the command says so.")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  drift import -spec ./openapi.json")
		fmt.Println("  drift import -spec https://api.example.com/openapi.json -project acme")
		os.Exit(1)
	}

	/* A relative -spec is resolved here, against the directory the operator is
	   standing in. If the running instance were left to resolve it, it would
	   resolve against its own working directory instead — a different answer to
	   the same path, and a confusing one, since the file the operator can see is
	   not the one that gets read. An absolute path is the same answer for both,
	   so the conversion happens before the request is sent. */
	source := *specPath
	if !openapi.LooksLikeURL(source) {
		abs, err := filepath.Abs(source)
		if err != nil {
			log.Fatalf("Failed to resolve spec path %q: %v", source, err)
		}
		source = abs
	}

	/* A running instance first, and by a wide margin.

	   This command used to write ~/.driftwood/baselines.json behind the back of
	   a proxy that held its own copy of the same document in memory: the next
	   save in the dashboard serialised that copy over the top and the import was
	   gone, with nothing said and every request still answering 200. Going
	   through the instance puts the contracts in the state the instance is
	   actually serving from, so there is no second writer and nothing to lose. */
	if controlURL, ok := liveInstance(); ok {
		importThroughInstance(controlURL, source, *projectFlag)
		return
	}

	// Load OpenAPI spec. One loader, which decides for itself which half of the
	// path-or-URL pair this source is — the same decision the running instance
	// makes for the same string, so the two channels cannot read different things
	// from one -spec.
	if openapi.LooksLikeURL(source) {
		log.Printf("[Driftwood] Loading OpenAPI spec from URL: %s", source)
	} else {
		log.Printf("[Driftwood] Loading OpenAPI spec from file: %s", source)
	}
	spec, err := openapi.LoadSpec(source)
	if err != nil {
		log.Fatalf("Failed to load OpenAPI spec: %v", err)
	}

	log.Printf("[Driftwood] OpenAPI spec loaded: %s v%s", spec.Info.Title, spec.Info.Version)

	/* Said here rather than at the top because everything above can still fail
	   without writing anything, and a warning about an overwrite followed by a
	   fatal about a typo'd path is two things to read where there is one problem.
	   This is the first point at which a store is definitely about to be opened
	   and written. */
	warnAboutDirectWrite()

	// Initialize storage with dummy target (we just need it for baseline storage).
	// A store that could not be read is reported and then written to anyway: this
	// command exists to put contracts in, and refusing to would leave the user
	// with neither their old contracts nor the imported ones.
	store, storeErr := storage.NewStore("http://localhost:3000", "8787")
	if storeErr != nil {
		log.Printf("[Driftwood] %v", storeErr)
	}

	// Import contracts.
	projectID, err := storage.ResolveImportProject(store, *projectFlag)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Printf("[Driftwood] Filing contracts under project %q", projectID)

	err = spec.ImportToStorage(projectID, store)
	if err != nil {
		log.Fatalf("Failed to import contracts: %v", err)
	}

	log.Println("[Driftwood] OpenAPI contracts imported successfully!")
	contracts, _ := spec.ExtractContracts()
	for _, c := range contracts {
		log.Printf("  %s %s (operation: %s)", c.Method, c.Path, c.OperationID)
	}
}

/* The channel that goes through a running instance.

   Both calls are deliberately short-timeout and non-retrying. The instance is
   on this machine — the hint file names loopback — so anything slower than a
   couple of seconds means it is not there in any sense worth waiting for, and
   the caller has a fallback for that. A long timeout would turn "no instance is
   running" into a command that appears to hang. */

// liveInstance reports where a running Driftwood is listening, if one is.
//
// The hint file says where an instance *was*. What makes this a fact is the
// second half: asking that address to identify itself, on a route nothing but
// this program serves. A stale hint — an instance killed without cleaning up,
// or a crash — costs one refused connection and lands the caller on the file
// path, which is the behaviour it had before any of this existed.
func liveInstance() (string, bool) {
	info, ok := storage.ReadInstance()
	if !ok {
		return "", false
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(info.ControlURL + proxy.ControlPrefix + "/api/instance")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.App != "driftwood" {
		return "", false
	}
	return info.ControlURL, true
}

// importThroughInstance asks the running instance to make the import.
//
// Every failure here is fatal rather than a fall back to writing the file. The
// caller has already established that an instance is running and owns the state;
// quietly writing the file anyway is the exact act whose consequences this whole
// path exists to avoid, and it would do it in the situation most likely to lose
// data. An error the operator can read is the smaller harm.
func importThroughInstance(controlURL, source, project string) {
	log.Printf("[Driftwood] A running instance answered at %s; asking it to make the import.", controlURL)

	payload, err := json.Marshal(struct {
		Spec    string `json:"spec"`
		Project string `json:"project"`
	}{Spec: source, Project: project})
	if err != nil {
		log.Fatalf("Failed to build the import request: %v", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(controlURL+proxy.ControlPrefix+"/api/import", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		log.Fatalf("A Driftwood instance is running at %s but the import request failed: %v\nThe instance's own copy of the store is the only one being served; stop it and run this again to write the file directly.", controlURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		/* Reached by something that answered the handshake but has no import
		   route. No build that exists today can do that — the two routes shipped
		   together — so this is a guard against the layout changing rather than a
		   case anyone is in. It is said separately because its remedy is "this
		   process is not what it says it is", which is not something the other
		   refusals should be read as. */
		log.Fatalf("Something at %s identified itself as Driftwood but has no import route. It is not a build this command knows how to import into; stop it and run this import again to write the store file directly.", controlURL)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Fatalf("The instance at %s refused the import: %s", controlURL, strings.TrimSpace(string(body)))
	}

	var result struct {
		Project  string `json:"project"`
		Imported []struct {
			Method      string `json:"method"`
			Path        string `json:"path"`
			OperationID string `json:"operation_id"`
		} `json:"imported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("The import was accepted but its reply could not be read: %v", err)
	}

	log.Printf("[Driftwood] OpenAPI contracts imported successfully into project %q.", result.Project)
	for _, c := range result.Imported {
		log.Printf("  %s %s (operation: %s)", c.Method, c.Path, c.OperationID)
	}
}

// warnAboutDirectWrite says what this command is about to do, and what will
// happen to it if the operator was wrong about nothing being up.
//
// It warns rather than refusing. No instance answering does not mean no
// instance exists — one on another machine, one whose hint file was never
// written because it was started by an older build — and a command that
// refused in those cases would be broken for the people whose setup is fine.
// The failure it is guarding against is silent, so saying it out loud is the
// whole difference.
func warnAboutDirectWrite() {
	storePath := filepath.Join(storage.PersistentDir(), "baselines.json")

	// The two cases are worded differently because they mean different things to
	// the operator. A hint that nothing answers is evidence something ran here
	// and died; no hint at all is the ordinary state of a machine where nothing
	// has. Same decision either way, different reason, and the reason is what
	// tells someone whether to go looking.
	if info, hinted := storage.ReadInstance(); hinted {
		log.Printf("[Driftwood] Nothing is answering at %s — the last instance to check in left that address behind — so this import writes %s directly.",
			info.ControlURL, storePath)
	} else {
		log.Printf("[Driftwood] No running Driftwood instance was found, so this import writes %s directly.", storePath)
	}

	log.Printf("[Driftwood] If an instance IS running — on another machine, or started before the address above was recorded — its next save writes its own view of the store over that file, and everything imported here is lost. Stop it first, or start it and run this again.")
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
	projectID := store.ActiveProject()
	if _, exists := store.GetBaseline(projectID, "GET", demoBaselinePath); exists {
		return
	}
	_, _ = store.SaveBaseline(projectID, "GET", demoBaselinePath, demoBaselinePayload)
}
