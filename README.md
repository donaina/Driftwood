# ⚡ APIDiff

> **Real-time API contract drift detector & lightweight sniffing proxy.**
> Protect your frontend from silent backend database type changes, breaking schema shifts, and field removals the millisecond they happen.

---

## 🚨 The Pain

The backend team changes a database column type or schema payload without notifying anyone. Your frontend state management or UI components silently break, and you waste hours debugging local state before realizing the raw network payload structure mutated.

## 💡 The Solution

**APIDiff** is a lightweight reverse proxy and local dashboard that sniffs local network traffic, infers JSON schema structures, locks baseline data contracts, and alerts you with visual structural diffs the instant an API contract breaks.

---

## ✨ Features

- 🔍 **Real-Time Traffic Sniffer**: Intercepts HTTP/JSON requests and responses without modifying payload data.
- 🧬 **Automatic Schema Extraction**: Infers full JSON schemas (primitives, nested objects, array element types, required keys).
- 🚨 **Millisecond Breaking Change Alerts**:
  - **Type Mutations**: Detects `integer` ➔ `string` (`99812` ➔ `"99812"`).
  - **Field Removals**: Flags missing required keys in response bodies.
  - **Nullability Violations**: Detects when non-null properties suddenly return `null`.
  - **Additive Changes**: Tracks newly introduced non-breaking properties.
- 📊 **Embedded Web Dashboard**: Native single-binary web interface accessible at `http://localhost:8787` with real-time SSE updates.
- 🧪 **Built-in Contract Simulator**: 1-click test triggers (`Type Mismatch`, `Removed Field`, `Nullability Violation`) to test contract alerts instantly.
- 💾 **Persistent Contract Storage**: Saved baseline contracts persist across restarts in `~/.apidiff/baselines.json`.

---

## 🚀 Quick Start

### Installation

```bash
git clone https://github.com/callmidavid/apidiff.git
cd apidiff
go build -o apidiff cmd/apidiff/main.go
```

### Running APIDiff

```bash
# Point APIDiff proxy to your target backend API (default target: http://localhost:3000)
./apidiff --port 8787 --target http://localhost:3000
```

- **Web Dashboard**: Open [http://localhost:8787](http://localhost:8787)
- **Proxy Endpoint**: Configure your frontend application API base URL to point to `http://localhost:8787` (all requests are proxied seamlessly to your target backend).

---

## 🛠 Is APIDiff Production-Ready?

**Yes for local development, staging environments, and CI/CD contract validation pipelines!**

### ✅ Production-Grade Capabilities
1. **Low Overhead**: Built with Go standard library `httputil.NewSingleHostReverseProxy` for ultra-low latency transparent proxying.
2. **Memory Safety**: Uses thread-safe mutex locking (`sync.RWMutex`) and a bounded ring buffer (500 requests max) to prevent memory leaks during high traffic load.
3. **Resilient SSE Streaming**: Non-blocking Server-Sent Events hub with drop safety ensures slow dashboard clients don't block API proxy throughput.
4. **Single Binary Deployment**: Zero runtime dependencies—the full Web Dashboard is compiled into the binary using `go:embed`.

### 📌 Roadmap & Future Enterprise Enhancements
- **Custom HTTPS/TLS Certificate Authority**: Support for system-wide HTTPS intercept proxying.
- **OpenAPI / Swagger Export**: Export detected baselines directly into OpenAPI 3.0 specs.
- **CI Pipeline Mode**: Flag `--fail-on-breaking` to exit with non-zero exit code when contract drift occurs in integration tests.

---

## 📐 Architecture

```
[ Frontend App ]
       │
       ▼
┌────────────────────────────────────────────────────────┐
│ APIDiff Proxy Server (Port 8787)                       │
│                                                        │
│  ├─ Proxy Interceptor  ──>  [ Target API Server ]      │
│  ├─ Schema Engine      ──>  Infer JSON Schema          │
│  ├─ Contract Diff      ──>  Compare vs Baseline        │
│  └─ Storage & SSE Hub  ──>  Broadcast Alerts           │
└────────────────────────────────────────────────────────┘
       │
       ▼
[ Web Dashboard & Diff Viewer ] (http://localhost:8787)
```

---

## 🧪 Testing

Run the internal unit test suite:

```bash
go test -v ./...
```

---

## 🤝 Contributing

Contributions are welcome! Please check out [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines and project structure details.

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
