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

.PHONY: all build dashboard deps test vet fmt fmt-check serve verify clean

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

# ------------------------------------------------------------------ gate

# The whole local gate. `dashboard` runs before `test` so the assertions below
# see a real build, and so a broken frontend build fails here rather than in a
# release.
verify: fmt-check vet dashboard test
	@test -f web/dist/assets/driftwood.css || { \
		echo "verify: web/dist/assets/driftwood.css is missing"; exit 1; }
	@n=$$(find web/dist -name '*.css' | wc -l | tr -d ' '); \
	if [ "$$n" != "1" ]; then \
		echo "verify: expected exactly one stylesheet, found $$n:"; \
		find web/dist -name '*.css'; exit 1; \
	fi
	@echo "verify: OK"

clean:
	rm -f $(BINARY)
	find web/dist -mindepth 1 ! -name PLACEHOLDER -delete
