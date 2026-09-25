package storage

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// captureLog runs fn with the standard logger redirected, and returns what was
// written.
//
// It restores the previous writer and flags, so a failure inside fn cannot
// leave every later test in this binary logging into a dead buffer. The suite
// has no t.Parallel(), which is the other half of why this is safe here: two
// tests swapping the global logger at once would interleave.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	fn()
	return buf.String()
}

// A file that changed underneath this process is reported before the save that
// replaces it.
//
// This is the backstop for the case the instance route does not cover: an
// import that never found the running proxy — an older build, a hand edit, an
// instance whose hint the importer could not read. The store cannot decide
// which of two complete documents should win, so it does the one thing that is
// unambiguously better than choosing quietly, and says so.
func TestForeignWriteIsReportedBeforeItIsOverwritten(t *testing.T) {
	s := testStore(t)
	path := s.persistPath

	// This process writes, so it has a stamp to compare against.
	if _, err := s.SaveBaseline(s.ActiveProject(), "GET", "/mine", `{"a": 1}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	// Something else writes the same file. A different size and a different
	// mtime, which is what a real second writer produces.
	foreign := []byte(`{"version": 4, "active_project": "default", "projects": [], "histories": {}, "thresholds": {}, "something_else": true}`)
	if err := os.WriteFile(path, foreign, 0600); err != nil {
		t.Fatalf("simulating a foreign write: %v", err)
	}

	out := captureLog(t, func() {
		if _, err := s.SaveBaseline(s.ActiveProject(), "GET", "/theirs", `{"b": 2}`); err != nil {
			t.Fatalf("SaveBaseline after the foreign write: %v", err)
		}
	})

	if !strings.Contains(out, path) {
		t.Errorf("the warning does not name the file, so it cannot be acted on:\n%s", out)
	}
	if !strings.Contains(out, "lost") {
		t.Errorf("the warning does not say the other writer's changes are gone, which is the part the operator needs:\n%s", out)
	}
}

// And it warns once per foreign write, not once per save.
//
// Every baseline save persists, so a warning on every save after the first
// would be a line per proxied endpoint — which is how a log becomes something
// people scroll past, and this warning is the whole of the backstop.
func TestForeignWriteIsReportedOnce(t *testing.T) {
	s := testStore(t)

	if _, err := s.SaveBaseline(s.ActiveProject(), "GET", "/mine", `{"a": 1}`); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	foreign := []byte(`{"version": 4, "active_project": "default", "projects": [], "histories": {}, "thresholds": {}, "something_else": true}`)
	if err := os.WriteFile(s.persistPath, foreign, 0600); err != nil {
		t.Fatalf("simulating a foreign write: %v", err)
	}

	out := captureLog(t, func() {
		for i := 0; i < 3; i++ {
			if _, err := s.SaveBaseline(s.ActiveProject(), "GET", "/theirs", `{"b": 2}`); err != nil {
				t.Fatalf("SaveBaseline %d: %v", i, err)
			}
		}
	})

	if n := strings.Count(out, "changed on disk"); n != 1 {
		t.Errorf("warned %d times across three saves, want once:\n%s", n, out)
	}
}

// This process's own writes are not reported as somebody else's.
//
// The stamp is recorded after every successful save, so an implementation that
// forgot to update it would warn on the very next save — a false alarm on the
// ordinary path, which is worse than no warning at all because it teaches the
// operator to ignore the one that is real.
func TestOurOwnWritesAreNotReported(t *testing.T) {
	s := testStore(t)

	out := captureLog(t, func() {
		for _, path := range []string{"/one", "/two", "/three"} {
			if _, err := s.SaveBaseline(s.ActiveProject(), "GET", path, `{"a": 1}`); err != nil {
				t.Fatalf("SaveBaseline(%s): %v", path, err)
			}
		}
	})

	if strings.Contains(out, "changed on disk") {
		t.Errorf("this process's own saves were reported as a foreign write:\n%s", out)
	}
}

// A store that has never written has no stamp, so the first save must not
// report anything: there was nothing here for anyone to have changed. This is
// every fresh install's first baseline.
func TestFirstSaveWithNothingOnDiskIsNotReported(t *testing.T) {
	s := testStore(t)

	out := captureLog(t, func() {
		if _, err := s.SaveBaseline(s.ActiveProject(), "GET", "/first", `{"a": 1}`); err != nil {
			t.Fatalf("SaveBaseline: %v", err)
		}
	})

	if strings.Contains(out, "changed on disk") {
		t.Errorf("the first save of a fresh install was reported as a foreign write:\n%s", out)
	}
}
