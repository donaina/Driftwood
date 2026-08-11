# APIDiff

## The Problem

The backend team changes a database type without telling anyone and your frontend silently breaks. You waste two hours debugging your state management before realizing the raw network payload changed.

## The Solution

A lightweight proxy that sniffs local network traffic, caches JSON payloads, and alerts the user with a clean diff the millisecond a data contract changes.

## How to Build It

Best built as a Chrome Extension inside DevTools, or a local Web Server (localhost dashboard).

## Project Structure

- `cmd/` - Command-line applications
  - `apidiff/` - Main application entry point
  - `app/` - App-related commands
- `internal/` - Private application code
  - `capture/` - Network traffic capture
  - `config/` - Configuration management
  - `contract/` - Data contracts
  - `diff/` - Diff calculation logic
  - `events/` - Event handling
  - `proxy/` - Proxy server implementation
  - `schema/` - Schema management
  - `server/` - HTTP server
  - `storage/` - Data storage
- `pkg/` - Public packages
  - `types/` - Shared types and contracts
- `web/` - Frontend application
- `docs/` - Documentation
- `tests/` - Integration tests

## Getting Started

```bash
go run cmd/apidiff/main.go
```

---

Monitor API contracts in real-time. Never get surprised by data structure changes again.
