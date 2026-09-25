# Driftwood

> **Real-time API contract drift detector & lightweight sniffing proxy.**
> Protect your frontend from silent backend database type changes, breaking schema shifts, and field removals the millisecond they happen.

---

## The Pain

The backend team changes a database column type or schema payload without notifying anyone. Your frontend state management or UI components silently break, and you waste hours debugging local state before realizing the raw network payload structure mutated.

## The Solution

**Driftwood** is a lightweight reverse proxy and local dashboard that sniffs local network traffic, infers JSON schema structures, locks baseline data contracts, and alerts you with visual structural diffs the instant an API contract breaks.

---

## Try it in two minutes

You do not need an API of your own, an account, or an API key. Driftwood ships a mock
endpoint with a baseline already seeded for it, so a fresh clone can watch a real
breaking change run through the real diff engine.

You need **Go 1.25 or newer** and **Node 20.19 or newer** (22 recommended) to build.
The Go side has no external dependencies, so `go build` needs no module downloads.

```bash
git clone https://github.com/donaina/Driftwood.git
cd Driftwood
make build     # installs frontend deps, builds the dashboard, then the binary
./drift --target http://localhost:3000
```

That target does not have to exist — the demo endpoint is served by Driftwood itself
and is intercepted before any dial, so nothing is ever sent to it. Naming one is what
tells Driftwood the setup is finished, so it skips the first-run wizard and connects
its live update stream. Started with no `--target` at all, Driftwood assumes
`http://localhost:3000` anyway, but still shows the wizard — and until you dismiss it,
the alert counter stays at zero and no toast appears, because the stream that drives
them has not opened yet.

Open **http://localhost:8787/_driftwood/** and use the **Contract Drift Simulator**
panel in the left sidebar:

| Click | What happens |
| --- | --- |
| **Normal Baseline** | The healthy response. Status `MATCH`, no alert. |
| **Type Mismatch** | `id` changes from `99812` to `"usr_99812"`. Status `BREAKING`, alert on `$.id`. |
| **Missing Field** | `email` disappears. Status `BREAKING`, alert on `$.email`. |
| **Null Violation** | `email` becomes `null`. Status `BREAKING`. |
| **Added Field** | A new key appears. Status `MATCH` — additive changes are tracked but never alerted on. |

Each button changes the mock endpoint's shape and then calls it, so the response is
diffed against the seeded baseline exactly as your own traffic would be. The breaking
ones raise a real alert: the counter at the top of the page increments, a toast
appears, and the record lands in **Contract Alerts** — click the counter to open that
view. Nothing here is staged for the demo; it is the same diff path a real request
takes.

One thing that surprises people: the **Trigger Test Payload** button in the header
re-sends the *current* mode rather than changing it, so pressing it repeatedly shows
you the same response. Use the sidebar buttons to switch modes.

To point Driftwood at an API of your own instead, see [Quick Start](#quick-start).

---

## Features

- **Real-Time Traffic Sniffer**: Intercepts HTTP/JSON requests and responses without modifying payload data.
- **Automatic Schema Extraction**: Infers full JSON schemas (primitives, nested objects, array element types, required keys).
- **Millisecond Breaking Change Alerts**:
  - **Type Mutations**: Detects `integer` ➔ `string` (`99812` ➔ `"99812"`).
  - **Field Removals**: Flags missing required keys in response bodies.
  - **Nullability Violations**: Detects when non-null properties suddenly return `null`.
  - **Format Changes**: Detects when a string's shape changes from one known format to another (`2024-01-31` ➔ a UUID), which a type check alone cannot see.
  - **Additive Changes**: Tracks newly introduced non-breaking properties.
- **AI Change Explanations (optional)**: A sidecar service reads the structural diff and writes a short explanation of it — what changed, what it is likely to break, and what to do next. It runs alongside Driftwood rather than inside it; alerts are complete without it, and the binary never waits on it.
- **TypeScript Type Exporter**: Generates `.d.ts` interface definitions directly from your baseline schemas.
- **JavaScript & Node.js Native Support**: Installable via `npx` / `npm`, with a module that starts and stops the proxy from a Node process.
- **Embedded Web Dashboard**: Native single-binary web interface at `http://localhost:8787/_driftwood/` with real-time SSE updates.
- **Built-in Contract Simulator**: 1-click test triggers (`Type Mismatch`, `Missing Field`, `Null Violation`) that drive the mock endpoint through a real breaking change, so you can watch an alert arrive end to end — see [Try it in two minutes](#try-it-in-two-minutes).
- **Persistent Contract Storage**: Saved baseline contracts persist across restarts in `~/.driftwood/baselines.json`.

---

## Severity model

Every difference between a response and its baseline is recorded as a **delta** on the traffic
record, and severity decides what happens beyond that. The one exception is a field whose name
marks it as volatile — a request ID, a trace ID, a timestamp — which is set aside before the
comparison. Its value changes on every call by design, so a difference there is not drift, and
a field that is nothing but noise cannot be reported as a broken contract.

| Severity | What it covers | What it does |
| --- | --- | --- |
| `BREAKING` | A required property is gone; a type changed; a non-nullable property returned `null`; a string's format changed from one known shape to another (`date` ➔ `uuid`). | Raises an alert and marks the request `BREAKING`. |
| `WARNING` | A property the baseline did not require is gone; `integer` widened to `number`; a value stopped matching any known format. | Marks the request `WARNING` and files it in the Alerts view. Nothing is broadcast — only `BREAKING` raises a live alert. |
| `INFO` | The response gained something: a new property, or a value that now matches a known format. Also a `null` that started returning a value, and a `number` that narrowed to whole numbers. | Counted as healthy — the request still reads `MATCH`. |

What raises an alert is configurable, per project, in **Alert Thresholds**: each kind of change has a
floor, the least severe difference of that kind worth telling you about. The table above describes
the default, which alerts on `BREAKING` and `WARNING` and ignores `INFO`. A lower floor files
informational changes in the Alerts view and delivers them to your webhooks; a higher one leaves only
`BREAKING` alerting.

A floor cannot change what a change *is*. Severity is measured from the diff, so no setting moves a
delta between the rows above — the dashboard can never show `MATCH` beside a `BREAKING` delta. The
live alert frame is fixed for the same reason: only `BREAKING` interrupts the dashboard, whatever
the floor says, because the floor answers "what should be recorded and sent" rather than "what
should stop what you are doing".

Whether a removed property is `BREAKING` or `WARNING` depends on what the baseline promised. A
contract imported from an OpenAPI document uses that document's `required` list, so a property
outside it is reported as a warning rather than as a broken promise. A baseline inferred from
traffic has a weaker claim to make: one response cannot distinguish a field the API always sends
from one it happened to send that day, so every key observed is treated as required.

An additive change keeps every promise the baseline made, so it is never a broken contract. At the
default floor it is tracked and shown but not alerted on — and only a floor lowered on purpose
makes it an alert.

---

## Quick Start

### 1. Go Lang

The dashboard is compiled into the binary, so it has to be built before the binary
that carries it:

```bash
git clone https://github.com/donaina/Driftwood.git
cd Driftwood
make build
./drift --port 8787 --target http://localhost:3000
```

`make build` installs the frontend dependencies, builds the dashboard, and then
builds the binary. By hand, that is:

```bash
npm --prefix frontend-react ci
npm --prefix frontend-react run build
go build -o drift ./cmd/drift
```

`web/web.go` embeds `web/dist`, so the binary serves its own dashboard from any
working directory. A committed `web/dist/PLACEHOLDER` keeps `//go:embed`
compiling on a checkout that has never run the frontend step — without it,
`//go:embed` is a compile error and a clean clone cannot build at all.

The cost is that the build succeeds either way, so the binary names the problem at
startup instead. If the assets are missing it says so, because otherwise every
asset URL answers `200` with the page's own HTML and the dashboard renders
unstyled and inert with nothing in the log to explain why.

### 2. JavaScript & TypeScript

> **Not on the npm registry yet.** `@donaina/driftwood` has never been published —
> the publish workflow runs when a GitHub Release is created, and GitHub Actions on
> this repository currently fails on a billing lock, so no release has ever been
> cut. The commands below are the intended interface, not a working one. Build from
> source above in the meantime.

#### Run directly via `npx`:

```bash
npx @donaina/driftwood --port 8787 --target http://localhost:3000
```

### Global Installation via NPM

```bash
npm install -g @donaina/driftwood
drift --port 8787 --target http://localhost:3000
```

### Programmable Integration in Node.js / Express

```ts
import Driftwood from "@donaina/driftwood";

const driftwood = new Driftwood({
  port: 8787,
  target: "http://localhost:3000",
});

await driftwood.start();
```

### 3. AI explanations (optional)

Detection and alerting need none of this. Every alert already carries the structural
diff; the sidecar adds a paragraph reading it back in prose, attached to the breaking
alerts on the Alerts view.

```bash
cd ai
npm ci
ANTHROPIC_API_KEY=sk-ant-... npm run dev     # listens on :8788
```

Or `make ai-serve` from the repository root, which runs `npm ci` first.

The sidecar listens on port **8788** and the proxy looks for it exactly there —
that address is a constant in `internal/proxy/proxy.go`, so leave `AI_PORT` alone
unless you also change it there.

Nothing tells Driftwood whether the sidecar is running, because nothing needs to.
If it is absent, breaking alerts are published immediately and without an
explanation; if it is present but slow, the alert has already gone out by the time
the explanation arrives, and the explanation is attached to the stored alert when it
lands. With no API key the service still starts and answers `503`, so

```bash
curl localhost:8788/health
```

tells you which state you are in rather than leaving you with a process that
silently is not working. `AI_MODEL` overrides the model it calls.

### 4. The website (optional)

`site/` is the marketing site and a try-it-out page, in a browser, for someone who
has not installed anything. It is a separate Vite build, embedded into the binary
the same way the dashboard is, so `make build` produces one file that carries both
surfaces and serving it needs no static host.

```bash
make site                      # → site/dist
npm --prefix site run preview
```

It shares the dashboard's token file (`frontend-react/src/tokens.css`) and nothing
else, so the two read as one product without the dashboard's shell — traffic table,
drawer, toasts, wizard — coming along. `make verify` builds it and asserts its CSS
actually carries those tokens: the import is a one-line change that would otherwise
fail silently, rendering the site with no colours at all and still exiting `0`.

**Serving it.** Off by default, because the site claims `/` and `/` otherwise belongs
to the target you are proxying:

```bash
./drift --site --port 8787 --target http://localhost:3000
```

`/` is then the landing page, `/try` the try-it-out page, and `/_driftwood/` still the
dashboard. `DRIFTWOOD_SITE=1` does the same thing for a container whose start command
cannot carry an argument; a `--site` you typed yourself outranks it, so
`--site=false` turns it back off. A build that never ran is warned about at startup
rather than left to be diagnosed as a routing bug — the same treatment `web/dist`
already gets.

**Run it against a live instance.** The try-it-out page drives the real control API
at `/_driftwood/*` and needs it same-origin, because the control plane's CORS
allowlist is localhost-only (`internal/server/server.go`, `isAllowedOrigin`). In dev,
`site/vite.config.ts` proxies that prefix to `127.0.0.1:8787`, so starting `./drift`
on its default port is enough:

```bash
./drift --port 8787 --target http://localhost:3000   # terminal 1
npm --prefix site run dev                            # terminal 2
```

With no instance reachable the page says so and shows the command to start one. It
does not fall back to canned output — a demo that quietly substitutes pre-recorded
results is worse than no demo, because the reader takes away a belief that nothing
they did established.

`Caddyfile.driftwood` is an example layout for a hand-managed host: everything to the
binary with buffering off, because `/events` is a long-lived SSE stream. This
project's own deployment is managed by Aeroplane, which generates its own config, so
nothing in that file needs editing to deploy.

**`railpack.json`.** The deployment build does not run `make` — it runs
[Railpack](https://railpack.com), which detects a Go project from `go.mod` and emits
`go build -o out ./cmd/drift` on its own. That build has no Node in it, so `web/dist`
and `site/dist` are still holding their placeholders when `//go:embed` runs, and the
binary ships with neither surface: every asset URL answers `200` with the page's own
HTML. `railpack.json` is an overlay on that generated plan — it adds Node, and puts the
two Vite builds ahead of the Go build. The `"..."` is the generated commands, expanded
where it appears, so the ordering is the whole point: `//go:embed` captures the
directory at compile time, and a Go build that runs first captures nothing. `make
build` remains the local equivalent, and `make verify` the gate.

---

## TypeScript Interface Generation

Driftwood automatically converts baseline API payload contracts into TypeScript type definitions:

- **Dashboard UI**: Click **`Export TypeScript Types (.d.ts)`** on [http://localhost:8787/_driftwood/](http://localhost:8787/_driftwood/).
- **HTTP Endpoint**: Download directly via `GET http://localhost:8787/_driftwood/api/export/typescript`.

Example output:

```typescript
// Auto-generated by Driftwood
export interface GetUsersResponse {
  email: string;
  id: number;
  is_active: boolean;
  roles: string[];
  score: number;
  username: string;
}
```

---

### Production-Grade Capabilities

1. **Low Overhead**: Built on Go's standard library `net/http/httputil.ReverseProxy`, with a custom `Rewrite` and transport for ultra-low latency transparent proxying.
2. **Memory Safety**: Uses thread-safe mutex locking (`sync.RWMutex`) and a bounded ring buffer (500 requests max) to prevent memory leaks under high traffic load.
3. **Resilient SSE Streaming**: Non-blocking Server-Sent Events hub with drop safety ensures slow dashboard clients don't block API proxy throughput.
4. **Single Binary Deployment**: Zero runtime dependencies — the dashboard page, `web/dist`, the marketing site and `site/dist` are all compiled into the binary with `go:embed`, so it serves both surfaces from any working directory and can be moved anywhere on its own.

---

## Project Directory Structure

```
.
├── ai/                  # Optional AI explainer sidecar (Node, port 8788)
├── bin/                 # Node.js CLI executable wrapper (npx support)
├── cmd/
│   └── drift/         # Main Go application entry point
├── docs/                # Architectural & schema diff specification docs
├── frontend-react/      # React view sources and the Vite build
│   └── src/             # Components, one entry per view the dashboard mounts
├── internal/
│   ├── capture/         # Network traffic payload & header sanitization
│   ├── contract/        # TypeScript interface generator & contract exports
│   ├── diff/            # Real-time JSON schema diffing engine & tests
│   ├── events/          # Server-Sent Events (SSE) broadcasting hub
│   ├── mock/            # Built-in interactive contract drift simulator
│   ├── openapi/         # OpenAPI spec import (`drift import -spec`)
│   ├── proxy/           # HTTP reverse proxy & traffic sniffing interceptor
│   ├── schema/          # Recursive JSON schema inference engine
│   ├── server/          # HTTP server router & REST API controllers
│   └── storage/         # Thread-safe in-memory store & disk persistence
├── pkg/
│   └── types/           # Core domain models (SchemaNode, ContractDiff, etc.)
├── site/                # Marketing site & try-it-out page (Vite build, embedded
│   └── src/             #   into the binary by site/site.go and served by it)
├── tests/               # End-to-end proxy integration tests
├── web/                 # Dashboard: index.html, shell.css, and the go:embed
│   └── dist/            #   Vite's output, embedded into the binary
├── index.js             # JavaScript/Node.js module export
├── index.d.ts           # TypeScript module declarations
├── Makefile             # build / test / serve / verify — the local gate
├── package.json         # NPM package metadata
├── CONTRIBUTING.md      # Developer contribution guide
└── LICENSE              # MIT License
```

The dashboard is a single `index.html` that mounts React views into itself, so the
Vite build in `frontend-react/` writes into `web/dist/` rather than a `dist/` of its
own. That is what lets `web/web.go` embed it — `//go:embed` cannot reach outside its
own package directory.

---

## Architecture

```
[ Frontend App ]
       │
       ▼
┌────────────────────────────────────────────────────────┐
│ Driftwood Proxy Server (Port 8787)                       │
│                                                        │
│  ├─ Proxy Interceptor  ──>  [ Target API Server ]      │
│  ├─ Schema Engine      ──>  Infer JSON Schema          │
│  ├─ Contract Diff      ──>  Compare vs Baseline        │
│  └─ Storage & SSE Hub  ──>  Broadcast Alerts           │
└────────────────────────────────────────────────────────┘
       │
       ▼
[ Web Dashboard & Diff Viewer ] (http://localhost:8787/_driftwood/)
```

### How requests are routed

Driftwood listens on one port and splits it by path:

| Path | Goes to |
| --- | --- |
| `/_driftwood/*` | The dashboard and its control API. Never proxied. |
| `/`, `/index.html`, `/try`, `/try.html`, `/assets/*`, `/favicon.svg`, `/dashboard-light.png`, `/dashboard-dark.png` | The marketing site — **only when started with `--site`**. |
| Everything else | Your target API — proxied, sniffed, diffed and recorded. |

The site is off by default and claiming `/` is the whole reason: on a machine where
you are developing against an app, `/` is that app's front page. The paths above are
claimed as a set and none of them is a prefix of another surface, so `/assets-old/x`
and `/pricing` still belong to the target. Inside the set a missing file is a `404`
— never the page, and never the proxy — because an asset URL that answers `200` with
HTML is the failure this project has already shipped once.

There is no `/api/` special case. Point your frontend at Driftwood instead of your
backend and every route it calls is observed, whether that is `/v1/orders`,
`/graphql` or `/api/users`. A path outside the paths above that the backend does not
recognise is answered by the backend, with the backend's own 404 — Driftwood does
not invent a response for it.

---

## Testing

```bash
make verify
```

That is the gate: it checks formatting, vets, builds the dashboard, runs the whole
test suite under the race detector, and asserts the build produced exactly one
stylesheet at the stable path `web/dist/assets/driftwood.css`. GitHub Actions on
this repository fails on a billing lock, so `make verify` is the check that
actually runs, not a wrapper around one.

The suite on its own:

```bash
make test          # go test ./... -race
make fmt           # gofmt -w over the Go source
```

---

## Contributing

Contributions are welcome! Please check out [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines and development workflow.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
