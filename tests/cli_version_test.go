package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/* `drift --version` is a one-line answer, and the Go side says so in a comment and
   pins it with a test. The npm wrapper printed a startup banner to stdout before
   spawning anything, so the answer a user actually got was two lines — and the
   one-line contract held only for a binary run directly, which is not how anyone
   installs it. `VERSION=$(drift --version)` captured the banner into the version.

   The wrapper is exercised for real rather than pattern-matched: child_process is
   stubbed through the module loader, so drift.js takes its own banner branch and
   its own argument parsing and spawns nothing. That keeps the assertion on the
   behaviour — what lands on stdout — instead of on the shape of the source. */

// cliHarness stubs child_process, restores argv to what the wrapper would see if
// node had been handed the script directly, then loads the real drift.js.
const cliHarness = `
const Module = require('module');
const orig = Module._load;

Module._load = function (request) {
  if (request === 'child_process') {
    return { spawn: () => ({ on() { return this; } }) };
  }
  return orig.apply(this, arguments);
};

// process.argv as drift.js expects it: [node, drift.js, ...userArgs].
const script = process.argv[2];
process.argv = [process.argv[0], script, ...process.argv.slice(3)];

require(script);
`

type cliResult struct {
	Stdout string
	Stderr string
	Code   int
}

func runCLIWrapper(t *testing.T, args ...string) cliResult {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on PATH; the gate builds the dashboard with npm, so this only skips outside it")
	}

	dir := t.TempDir()
	harness := filepath.Join(dir, "cli-harness.js")
	if err := os.WriteFile(harness, []byte(cliHarness), 0o644); err != nil {
		t.Fatalf("writing the harness: %v", err)
	}
	wrapper, err := filepath.Abs(filepath.Join("..", "bin", "drift.js"))
	if err != nil {
		t.Fatalf("resolving bin/drift.js: %v", err)
	}

	cmd := exec.Command(node, append([]string{harness, wrapper}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	// A non-zero exit is reported rather than failed on, so a test can assert
	// about it. The stubbed child never emits, so the wrapper should exit 0.
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running the wrapper: %v", err)
	}

	return cliResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

func TestCLIVersionAnswerIsOneLine(t *testing.T) {
	for _, flag := range []string{"--version", "-version"} {
		t.Run(flag, func(t *testing.T) {
			got := runCLIWrapper(t, flag)

			if strings.Contains(got.Stdout, "Starting Driftwood proxy") {
				t.Errorf("`drift %s` printed the startup banner on stdout, so the answer is not one line:\n%s", flag, got.Stdout)
			}
			// One line, and the empty string when nothing was printed at all —
			// splitting "" gives [""], which would pass a naive count.
			if trimmed := strings.TrimRight(got.Stdout, "\n"); trimmed != "" {
				t.Errorf("`drift %s` wrote %q to stdout; a version query is answered by the binary, not the wrapper", flag, trimmed)
			}
			if got.Code != 0 {
				t.Errorf("exit code = %d, want 0", got.Code)
			}
		})
	}
}

// Suppressing the banner must not suppress it for a real start, which is the
// only thing the banner is for.
func TestCLIStillAnnouncesAStart(t *testing.T) {
	got := runCLIWrapper(t)

	if !strings.Contains(got.Stdout, "Starting Driftwood proxy") {
		t.Errorf("a plain `drift` no longer announces itself:\n%s", got.Stdout)
	}
}
