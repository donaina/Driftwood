package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

/* Three separate edits make a view reachable, and they agree only by
   convention: a sidebar <button id="nav-X">, a destination for the tab (either
   an entry in switchTab's reactViews table, which names a show*Library function
   that reveals a *-react-root, or a .view-panel named view-X), and an
   on('nav-X', …) handler that calls switchTab in the first place.

   Nothing in the build knows about any of them. `go build` does not read
   web/index.html, `tsc -b && vite build` compiles the React island without
   knowing which id mounts it, and the stylesheet has never seen either. So a
   half-done route ships as a nav item that does nothing when clicked, and it
   ships green: every existing check passes, because every existing check is
   looking somewhere else.

   That is not hypothetical. This test would have caught the shape of two
   defects already in this file's history — a reactViews entry added before the
   show*Library function it names existed, and a show*Library function that
   stripped .active from every nav item and re-added nothing, undoing the class
   switchTab had just set.

   There is no frontend test runner to put this in. vitest is a devDependency
   with zero test files, and `make verify` runs `tsc -b && vite build` for the
   dashboard — a type-check and a bundle, neither of which can see a string
   literal in an HTML file. So the assertion lives here, in Go, where the gate
   actually runs it. */

var (
	navID      = regexp.MustCompile(`id="nav-([a-z0-9-]+)"`)
	viewID     = regexp.MustCompile(`id="view-([a-z0-9-]+)"`)
	navHandler = regexp.MustCompile(`on\('nav-([a-z0-9-]+)'`)
	viewKey    = regexp.MustCompile(`'([a-z0-9-]+)'\s*:`)
)

// shellSource reads the dashboard shell. The path is relative to this package
// because `go test` runs each package with its own directory as the working
// directory.
func shellSource(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "web", "index.html")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}

// reactViewsKeys pulls the keys out of switchTab's reactViews table.
//
// Parsed by locating the literal and cutting to its closing brace rather than by
// matching the table's indentation, which gofmt does not own and which nothing
// would fail on if it moved.
func reactViewsKeys(t *testing.T, src string) []string {
	t.Helper()

	_, rest, ok := strings.Cut(src, "const reactViews = {")
	if !ok {
		t.Fatal("web/index.html has no `const reactViews = {` — switchTab's table was renamed or removed, and this test can no longer see the routing it asserts")
	}

	block, _, ok := strings.Cut(rest, "};")
	if !ok {
		t.Fatal("the reactViews table is not closed by `};`")
	}

	var keys []string
	for _, m := range viewKey.FindAllStringSubmatch(block, -1) {
		keys = append(keys, m[1])
	}
	if len(keys) == 0 {
		t.Fatal("the reactViews table has no keys — the parse found the literal but nothing in it")
	}
	return keys
}

func matchSet(re *regexp.Regexp, src string) map[string]bool {
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		found[m[1]] = true
	}
	return found
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// TestEveryReactViewIsReachableByNav checks that every React destination is
// wired end to end.
//
// A reactViews key is the shell's own declaration that a tab exists, so each one
// must have a sidebar button to reach it and a handler on that button — the
// table is not consulted at all unless on('nav-X') calls switchTab('X').
func TestEveryReactViewIsReachableByNav(t *testing.T) {
	src := shellSource(t)
	navs := matchSet(navID, src)
	handlers := matchSet(navHandler, src)

	for _, name := range reactViewsKeys(t, src) {
		if !navs[name] {
			t.Errorf("reactViews has a %q entry but web/index.html has no `id=\"nav-%s\"`, so nothing can switch to it", name, name)
		}
		if !handlers[name] {
			t.Errorf("reactViews has a %q entry but no `on('nav-%s', …)` handler, so it is a view with a sidebar button that does nothing when clicked", name, name)
		}
	}
}

// TestEveryNavItemHasADestination checks the converse direction: every sidebar
// button goes somewhere.
//
// A nav item with no reactViews entry is only correct if it has a .view-panel of
// its own to activate, because switchTab refuses a destination it cannot find and
// logs "navigation ignored" — which is a correct refusal, and also a button that
// does nothing when clicked.
func TestEveryNavItemHasADestination(t *testing.T) {
	src := shellSource(t)
	navs := matchSet(navID, src)
	handlers := matchSet(navHandler, src)
	views := matchSet(viewID, src)
	reactKeys := map[string]bool{}
	for _, name := range reactViewsKeys(t, src) {
		reactKeys[name] = true
	}

	for _, name := range sorted(navs) {
		if !handlers[name] {
			t.Errorf("web/index.html has `id=\"nav-%s\"` with no `on('nav-%s', …)` handler — the button renders and clicks into nothing", name, name)
		}
		if reactKeys[name] || views[name] {
			continue
		}
		t.Errorf("web/index.html has `id=\"nav-%s\"` but no destination for it: it is neither a reactViews key nor paired with `id=\"view-%s\"`, so switchTab logs \"navigation ignored\" and leaves the previous view on screen", name, name)
	}
}
