# Driftwood Project — Conversation History Summary

**Date:** 2026-08-16  
**Repo:** `donaina/Driftwood` (fork of `callmidavid/apidiff`)  
**Local:** `~/Dev/driftwood`

---

## Project Overview

Driftwood — a refined rebuild of `callmidavid/apidiff`: an API contract-drift detector (reverse proxy that sniffs JSON responses, infers per-endpoint JSON schemas, locks baselines, alerts on drift via SSE dashboard).

**Stack:** Go 1.25 core (proxy, schema, diff) + TypeScript dashboard (go:embed) + npm distribution (`@donaina/driftwood`)

---

## Completed PRs (Merged)

| PR | Title | Key Changes |
|----|-------|-------------|
| **#1** | Rebrand apidiff → Driftwood | Module `github.com/donaina/driftwood`, binary `drift`, routes `/_driftwood`, dir `~/.driftwood`, CI gate |
| **#2** | Diff engine correctness | 8 bugs fixed: integer↔number both dirs, nullability fall-through, null→typed=INFO, empty-array unknown, deterministic ordering, JSONPath quoting, WARNING tier, structural deltas |
| **#3** | Security hardening | SSRF protection, secret sanitization (headers+body), CORS localhost-only, 127.0.0.1 bind, 0700/0600 perms, atomic write, SHA256 binary verification |
| **#4** | Schema inference + formats | Defensive copy in mergeNodes (#31), format detection: date/date-time/uuid/email (#34) |
| **#5** | Persistence + history | EndpointHistory with versioned baselines, ring buffers, atomic write, corrupt backup, deep copies, seed-if-absent, frequency-based required keys foundation |
| **#6** | Proxy hot-path robustness | gzip decompression, 10MB streaming limit, DevMockMode flag, 502 on target failure, 204 capture, SSE alert drop counter |
| **#7** | TypeScript generator | Quoted reserved/invalid keys, nested object extraction (User/Metadata/ItemsItem), optional `?` for nullable, array item interfaces, dedup names |
| **#8** | OpenAPI import | `drift import -spec <file\|url>` — loads OpenAPI 3.x, resolves $ref, allOf/anyOf/oneOf, generates samples |

---

## Open PRs

| PR | Status | Description |
|----|--------|-------------|
| **#9** | **OPEN** | AI diff explanations — TypeScript sidecar (port 8788) with `/explain`, Go proxy async call |
| **#10** | **PLANNED** | Dashboard refresh — beautifului.dev design, history browser, AI explanation panel |

---

## Key Technical Decisions

- **Clean start:** Repo rewritten to single initial commit `d0965b8` — no trace of `callmidavid/apidiff`
- **CI blocked:** GitHub Actions hold on `donaina` account (billing verification needed) — local gate is authoritative
- **No Claude attribution:** No `Co-Authored-By` on commits, no "Generated with Claude Code" in PRs
- **Branch → PR → merge:** Always PR, never direct to main
- **Test-first:** Each PR adds reproducing tests before fixes

---

## Local Green Gate (Authoritative)
```bash
go build ./... && go vet ./... && go test ./... -race  # ✅ All pass
```

---

## Repo State
- **Remote:** `origin` = `github.com/donaina/Driftwood` (only `main` branch exists)
- **Upstream:** `callmidavid/apidiff` (sync only, never push)
- **Author:** Ayoola Aina <ayo@webraiders.co>

---

## Next Steps
1. Wait for GitHub Actions billing verification (or accept local-only CI)
2. PR #9: Merge AI explanations once sidecar tested
3. PR #10: Dashboard refresh (decide: single-file vs React build)
4. Release: Tag `v1.0.0` → auto binaries + npm publish

---

## Memory Files
- `memory/driftwood-project.md` — project status & PR sequence
- `memory/git-workflow.md` — git/PR rules
- `memory/driftwood-ci-blocked.md` — CI hold documentation