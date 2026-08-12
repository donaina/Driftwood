# Contributing to Driftwood

Thank you for your interest in improving Driftwood! We welcome contributions from engineers and developers.

## How to Contribute

### 1. Reporting Bugs & Feature Proposals
- Check open issues to see if your bug or feature request has already been reported.
- Open a detailed issue describing the bug, step-to-reproduce, or proposed architecture improvement.

### 2. Local Setup & Testing
1. Clone the repository:
   ```bash
   git clone https://github.com/donaina/Driftwood.git
   cd Driftwood
   ```
2. Run the test suite:
   ```bash
   go test -v ./...
   ```
3. Build and test locally:
   ```bash
   go run cmd/drift/main.go --port 8787
   ```

### 3. Submission Guidelines
- Keep pull requests focused on a single logical change.
- Ensure all Go tests pass before submitting.
- Follow Go standard formatting (`gofmt`).
- Write tests for new schema inference or contract diffing edge cases.

## Development Architecture Overview

- **`cmd/drift/`**: CLI entrypoint.
- **`internal/proxy/`**: HTTP reverse proxy with traffic sniffing interceptor.
- **`internal/schema/`**: Recursive JSON schema inference engine.
- **`internal/diff/`**: Structural diffing engine & contract violation rules.
- **`internal/storage/`**: Thread-safe memory buffer & `.driftwood/baselines.json` persistence.
- **`internal/events/`**: Real-time SSE broadcasting hub.
- **`internal/mock/`**: Interactive mock backend simulator for testing contract drift.
- **`web/`**: Single-page Web Dashboard embedded via `go:embed`.

Thank you for helping keep API contracts healthy! ⚡
