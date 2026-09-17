# Driftwood

> **Real-time API contract drift detector & lightweight sniffing proxy.**
> Protect your frontend from silent backend database type changes, breaking schema shifts, and field removals the millisecond they happen.

---

## The Pain

The backend team changes a database column type or schema payload without notifying anyone. Your frontend state management or UI components silently break, and you waste hours debugging local state before realizing the raw network payload structure mutated.

## The Solution

**Driftwood** is a lightweight reverse proxy and local dashboard that sniffs local network traffic, infers JSON schema structures, locks baseline data contracts, and alerts you with visual structural diffs the instant an API contract breaks.

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
- **TypeScript Type Exporter**: Generates `.d.ts` interface definitions directly from locked baseline schemas.
- **JavaScript & Node.js Native Support**: Installable via `npx` / `npm`, with a module that starts and stops the proxy from a Node process.
- **Embedded Web Dashboard**: Native single-binary web interface at `http://localhost:8787/_driftwood/` with real-time SSE updates.
- **Built-in Contract Simulator**: 1-click test triggers (`Type Mismatch`, `Removed Field`, `Nullability Violation`) to test contract alerts instantly.
- **Persistent Contract Storage**: Saved baseline contracts persist across restarts in `~/.driftwood/baselines.json`.

---

## Severity model

Every difference between a response and its baseline is recorded as a **delta** on the traffic
record, so nothing is dropped. Severity decides what happens beyond that:

| Severity | What it covers | What it does |
| --- | --- | --- |
| `BREAKING` | A required property is gone; a type changed; a non-nullable property returned `null`; a string's format changed from one known shape to another (`date` ➔ `uuid`). | Raises an alert and marks the request `BREAKING`. |
| `WARNING` | A property the baseline did not require is gone; `integer` widened to `number`; a value stopped matching any known format. | Marks the request `WARNING`. Does not alert. |
| `INFO` | The response gained something: a new property, or a value that now matches a known format. | Counted as healthy — the request still reads `MATCH`. |

Whether a removed property is `BREAKING` or `WARNING` depends on what the baseline promised. A
contract imported from an OpenAPI document uses that document's `required` list, so a property
outside it is reported as a warning rather than as a broken promise. A baseline inferred from
traffic has a weaker claim to make: one response cannot distinguish a field the API always sends
from one it happened to send that day, so every key observed is treated as required.

An additive change keeps every promise the baseline made, so it is tracked and shown but never
alerted on.

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

Skipping the frontend step fails the build rather than surprising you at runtime:
`web/web.go` embeds `web/dist`, and `//go:embed` is a compile error when its
directory is missing.

### 2. JavaScript & TypeScript

> **Not on the npm registry yet.** `@donaina/driftwood` has never been published —
> the release workflow needs a tag pushed, and GitHub Actions on this repository
> currently fails on a billing lock, so no release has ever been cut. The commands
> below are the intended interface, not a working one. Build from source above in
> the meantime.

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

---

## TypeScript Interface Generation

Driftwood automatically converts locked baseline API payload contracts into TypeScript type definitions:

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

1. **Low Overhead**: Built with Go standard library `httputil.NewSingleHostReverseProxy` for ultra-low latency transparent proxying.
2. **Memory Safety**: Uses thread-safe mutex locking (`sync.RWMutex`) and a bounded ring buffer (500 requests max) to prevent memory leaks under high traffic load.
3. **Resilient SSE Streaming**: Non-blocking Server-Sent Events hub with drop safety ensures slow dashboard clients don't block API proxy throughput.
4. **Single Binary Deployment**: Zero runtime dependencies — the dashboard page and `web/dist` are compiled into the binary with `go:embed`, so it serves its own UI from any working directory and can be moved anywhere on its own.

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
│   ├── proxy/           # HTTP reverse proxy & traffic sniffing interceptor
│   ├── schema/          # Recursive JSON schema inference engine
│   ├── server/          # HTTP server router & REST API controllers
│   └── storage/         # Thread-safe in-memory store & disk persistence
├── pkg/
│   └── types/           # Core domain models (SchemaNode, ContractDiff, etc.)
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
| Everything else | Your target API — proxied, sniffed, diffed and recorded. |

There is no `/api/` special case. Point your frontend at Driftwood instead of your
backend and every route it calls is observed, whether that is `/v1/orders`,
`/graphql` or `/api/users`. A path outside `/_driftwood/` that the backend does not
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
