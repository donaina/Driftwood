# Contributing to Driftwood

Thank you for your interest in improving Driftwood! We welcome contributions from engineers and developers.

## Toolchain floors

| | Version | Where it is declared |
|---|---|---|
| Go | 1.25.0 | `go.mod` |
| Node | `^20.19.0 \|\| >=22.12.0` | `frontend-react/package.json`, `site/package.json` |

The dashboard and the marketing site are both Vite 8 applications and both are **embedded into the
Go binary** (`web/web.go`, `site/site.go`). Vite 8 is what sets the Node floor above; the root
`package.json`'s `>=16.0.0` describes the published npm wrapper, which is not what builds these.

## How to Contribute

### 1. Reporting Bugs & Feature Proposals
- Check open issues to see if your bug or feature request has already been reported.
- Open a detailed issue describing the bug, step-to-reproduce, or proposed architecture improvement.

### 2. Local Setup & Testing

```bash
git clone https://github.com/donaina/Driftwood.git
cd Driftwood
make verify
```

**`make verify` is the gate.** It runs `fmt-check vet dashboard site pack-check test`, where `test`
is `go test ./... -race`, and then asserts what the build produced: exactly one stylesheet in
`web/dist` and in `site/dist`, each carrying the shared tokens. Run it before you open a pull
request, because nothing else will.

Two things about it are worth knowing up front:

- **`dashboard` and `site` come first for a reason.** `//go:embed` is a compile error when its
  directory is missing, and a Vite build that drops a stylesheet still exits 0. A checkout that has
  never built a frontend cannot even be vetted, and one that built badly passes every Go check while
  shipping an unstyled page.
- **Actions never runs this.** CI is billing-locked on this repository, so a red or absent CI run
  means nothing here. `make verify` is the gate itself, not a wrapper around one.

`make serve` builds both frontends and then runs the proxy against `http://localhost:3000`. Prefer
it to `go run cmd/drift/main.go`, which **does not build either Vite app** — it runs against the
committed `web/dist/PLACEHOLDER` and serves a dashboard with no CSS and no JavaScript, which looks
like a broken application rather than a missing build step.

**Isolate `HOME` when you run it.** Driftwood is a reverse proxy: it stores what it sees in
`~/.driftwood/`, and a run against real traffic writes real request data there. `make build` then
`./drift` directly is the way to do that, since `make serve` runs under your own `$HOME`:

```bash
make build
HOME=$(mktemp -d) ./drift --port 8787 --target http://localhost:3000
```

### 3. Submission Guidelines
- Keep pull requests focused on a single logical change.
- **Open a pull request; do not commit to `main`.** Merging is the maintainer's call.
- Run `make verify` and make sure it is green.
- Format Go with `gofmt` (`make fmt`), which covers `GO_DIRS := cmd internal pkg site web tests` —
  scoped rather than `.` so that `gofmt` does not walk `frontend-react/node_modules`.
- Write tests for new schema inference or contract diffing edge cases.

## Development Architecture Overview

- **`cmd/drift/`**: the CLI, which has exactly two modes — the default one, which starts the proxy
  and the dashboard, and `drift import`, which files an OpenAPI document's contracts into a project.
- **`internal/proxy/`**: the reverse proxy, its traffic-sniffing interceptor, and the `/_driftwood`
  control plane.
- **`internal/server/`**: the dashboard's HTTP surface — control routes, SSE wiring, and the
  loopback check (`isLoopbackRequest`) that is this product's only trust boundary.
- **`internal/capture/`**: redaction at the capture boundary — the patterns that keep a stored
  response from carrying a token or an email into the store.
- **`internal/schema/`**: recursive JSON schema inference.
- **`internal/diff/`**: the structural diff engine and contract violation rules — the seven delta
  kinds and the severity it measures for each.
- **`internal/contract/`**: turning a locked baseline into an artifact (TypeScript interfaces).
- **`internal/openapi/`**: loading an OpenAPI document from a path or a URL, and extracting the
  contracts it describes.
- **`internal/storage/`**: the thread-safe in-memory store and its persistence to
  `~/.driftwood/baselines.json`. Per-project histories, baselines, thresholds and webhooks all live
  in that one document.
- **`internal/events/`**: the SSE broadcasting hub.
- **`internal/webhook/`**: outbound alert delivery to Slack, Teams, Discord and generic endpoints.
- **`internal/netguard/`**: the SSRF guard shared by the deliverer and the spec fetcher, so the
  proxy and the webhooks cannot disagree about which addresses are reachable.
- **`internal/mock/`**: the interactive mock backend simulator for testing contract drift.
- **`pkg/types/`**: the wire types — schemas, deltas, alert and threshold config.
- **`web/`**: the dashboard — the shell (`index.html`, `shell.css`) plus the embedded Vite build.
- **`frontend-react/`**: the React 19 / Tailwind 4 components the shell mounts, and `tokens.css`,
  which is the single source of every colour on both surfaces.
- **`site/`**: the marketing site, a second Vite build embedded by `site/site.go`.
- **`ai/`**: the optional explanation sidecar. Not part of `make build` or `make verify`: a missing
  sidecar costs one refused connection and one paragraph, and a gate needing an API key is not a
  gate.
- **`bin/`**: the npm wrapper — `install.js` fetches a release binary, `drift.js` runs it, and
  `check-pack.js` asserts what `npm pack` would publish.
- **`tests/`**: cross-package tests that read the shipped shell rather than a package's own code.

Thank you for helping keep API contracts healthy! ⚡
