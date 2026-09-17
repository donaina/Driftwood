# Driftwood's build.
#
# `go build` alone is no longer enough. web/web.go embeds web/dist, so the Go
# build depends on the Vite build having run first — and because //go:embed is a
# compile error when its directory is missing, a clone that has never run a
# frontend build cannot even be vetted or tested. That ordering is what this
# file exists to hold; without it every entry point has to remember it.
#
# CI has never run successfully on this repository (the Actions jobs fail on a
# billing lock), so `make verify` is the gate, not a convenience wrapper.

NPM   := npm --prefix frontend-react
BINARY := drift
TARGET ?= http://localhost:3000
PORT   ?= 8787

# Every directory that holds Go source. Scoped rather than `.` so gofmt does not
# walk frontend-react/node_modules.
GO_DIRS := cmd internal pkg web tests

.PHONY: all build dashboard deps test vet fmt fmt-check serve verify clean \
        site site-deps ai-deps ai-build ai-serve

all: build

# ---------------------------------------------------------------- dashboard

deps: frontend-react/node_modules

# Tied to the lockfile: npm ci deletes and reinstalls node_modules, so running
# it on every build would add half a minute to each one for no gain.
frontend-react/node_modules: frontend-react/package-lock.json
	$(NPM) ci
	@touch $@

# Cleared before writing, but the committed PLACEHOLDER is kept: Vite is
# configured not to empty this directory precisely so that file survives, and
# deleting it here would undo that.
dashboard: deps
	@find web/dist -mindepth 1 ! -name PLACEHOLDER -delete
	$(NPM) run build
	@test -f web/dist/assets/driftwood.css || { \
		echo "dashboard: web/dist/assets/driftwood.css was not produced"; exit 1; }

# --------------------------------------------------------------------- go

build: dashboard
	go build -o $(BINARY) ./cmd/drift

serve: build
	./$(BINARY) --port $(PORT) --target $(TARGET)

test:
	go test ./... -race

vet:
	go vet ./...

fmt:
	gofmt -w $(GO_DIRS)

# 19 files at HEAD were unformatted and had always been, because nothing ever
# checked. This is the check that keeps that from recurring.
fmt-check:
	@out=$$(gofmt -l $(GO_DIRS)); \
	if [ -n "$$out" ]; then \
		echo "fmt-check: these files are not gofmt'd:"; echo "$$out"; \
		echo "run: make fmt"; exit 1; \
	fi

# ------------------------------------------------------------------- site

# The marketing site: its own Vite build, deployed independently of the binary.
#
# It shares the dashboard's token file but nothing else, so it builds on its own
# and cannot break the embedded dashboard by changing. `emptyOutDir` is on in
# its Vite config (unlike the dashboard's, which keeps a committed PLACEHOLDER
# alive), so there is nothing to clear here first.
#
# Both entries are asserted because the failure mode is quiet: a config change
# that drops one of them still exits 0 and still writes index.html, and the only
# symptom is that /try.html 404s in production.
SITE_NPM := npm --prefix site

site-deps: site/node_modules

# Tied to the lockfile, for the same reason the dashboard's is: npm ci throws
# node_modules away first, so running it every time would add half a minute to
# each build for no gain.
site/node_modules: site/package-lock.json
	$(SITE_NPM) ci
	@touch $@

site: site-deps
	$(SITE_NPM) run build
	@for page in index.html try.html; do \
		test -f site/dist/$$page || { \
			echo "site: site/dist/$$page was not produced"; exit 1; }; \
	done
	@echo "site: OK"

# ------------------------------------------------------------------ gate

# The whole local gate. `dashboard` runs before `test` so the assertions below
# see a real build, and so a broken frontend build fails here rather than in a
# release. `site` is in here for the same reason: it is shipped code, and a site
# that builds to an unstyled page still exits 0.
verify: fmt-check vet dashboard site test
	@test -f web/dist/assets/driftwood.css || { \
		echo "verify: web/dist/assets/driftwood.css is missing"; exit 1; }
	@n=$$(find web/dist -name '*.css' | wc -l | tr -d ' '); \
	if [ "$$n" != "1" ]; then \
		echo "verify: expected exactly one dashboard stylesheet, found $$n:"; \
		find web/dist -name '*.css'; exit 1; \
	fi
	@n=$$(find site/dist -name '*.css' | wc -l | tr -d ' '); \
	if [ "$$n" != "1" ]; then \
		echo "verify: expected exactly one site stylesheet, found $$n:"; \
		find site/dist -name '*.css'; exit 1; \
	fi
	@for pair in "web/dist|the dashboard's" "site/dist|the site's"; do \
		dir=$${pair%%|*}; who=$${pair#*|}; \
		css=$$(find $$dir -name '*.css'); \
		grep -q -- '--dw-light-bg-main' "$$css" || { \
			echo "verify: $$who CSS carries none of the shared tokens."; \
			echo "        $$dir was built from a stylesheet that no longer"; \
			echo "        imports frontend-react/src/tokens.css, so it would"; \
			echo "        render with no colours at all — and exit 0."; \
			exit 1; }; \
	done
	@echo "verify: OK"

# ---------------------------------------------------------------- ai sidecar

# Optional, and deliberately not part of `build` or `verify`. Driftwood is
# complete without it: an absent sidecar costs one refused connection and a
# missing paragraph, and nothing in the proxy waits on it. A gate that needed an
# ANTHROPIC_API_KEY would not be a gate anyone could run.
AI_NPM := npm --prefix ai

ai-deps: ai/node_modules

ai/node_modules: ai/package-lock.json
	$(AI_NPM) ci
	@touch $@

ai-build: ai-deps
	$(AI_NPM) run build

# Reads ANTHROPIC_API_KEY at startup, and AI_MODEL if you want a model other
# than the default. Without a key the service still starts and answers 503 in
# degraded mode, so a missing key is visible in GET /health rather than fatal.
ai-serve: ai-deps
	$(AI_NPM) run dev

clean:
	rm -f $(BINARY)
	find web/dist -mindepth 1 ! -name PLACEHOLDER -delete
