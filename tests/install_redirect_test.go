package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

/* GitHub serves a release asset with a 302 to objects.githubusercontent.com, and
   `bin/install.js` fetched its checksums with a helper that accepted only 200.
   Every install therefore threw "Checksums HTTP 302" — and because that fetch
   shares a try block with the binary download, it took the binary down with it.

   The result was quiet in the way this codebase cares about. `npm install -g`
   printed success, and what it left behind was a binary built from source by the
   local Go toolchain, which answers `drift --version` with "dev" because no
   release workflow stamped it. So the install appeared to work, reported a
   version, and was not the release — on a machine with Go. On a machine without
   it, nothing was installed at all.

   Nothing else could catch it. Both fetches work against a browser and against
   curl -L, the file is valid either way, and the failure is swallowed by a
   fallback that succeeds. So the assertion is behavioural: it drives the real
   downloadChecksums over a stubbed transport and holds it to the redirect.

   install.js is requireable only because its install() call is guarded by
   require.main === module; without that the require below would run a real
   install. */

// installHarness is the node side of the test: it replaces https.get with a
// queue of canned responses, then calls the real downloadChecksums and reports
// what came back.
const installHarness = `
const https = require('https');
const queue = JSON.parse(process.argv[2]);
const seen = [];

https.get = function (url, cb) {
  seen.push(url);
  const r = queue.shift();
  if (!r) throw new Error('unexpected extra request: ' + url);
  const res = {
    statusCode: r.statusCode,
    headers: r.headers || {},
    resume() {},
    setEncoding() {},
    on(ev, fn) {
      // Handlers attach synchronously inside the get callback, so the body is
      // delivered during attachment rather than on a later tick.
      if (ev === 'data' && r.body) fn(r.body);
      if (ev === 'end') fn();
      return this;
    },
  };
  process.nextTick(() => cb(res));
  return { on() { return this; }, setTimeout() { return this; }, destroy() {} };
};

require(process.argv[3]).downloadChecksums('1.0.0')
  .then(sums => console.log(JSON.stringify({ ok: true, seen, sums })))
  .catch(err => console.log(JSON.stringify({ ok: false, seen, error: err.message })));
`

type installResult struct {
	OK    bool              `json:"ok"`
	Seen  []string          `json:"seen"`
	Sums  map[string]string `json:"sums"`
	Error string            `json:"error"`
}

func runInstallHarness(t *testing.T, responses string) installResult {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on PATH; the gate builds the dashboard with npm, so this only skips outside it")
	}

	dir := t.TempDir()
	harness := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(harness, []byte(installHarness), 0o644); err != nil {
		t.Fatalf("writing the harness: %v", err)
	}
	install, err := filepath.Abs(filepath.Join("..", "bin", "install.js"))
	if err != nil {
		t.Fatalf("resolving bin/install.js: %v", err)
	}

	cmd := exec.Command(node, harness, responses, install)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the harness exited non-zero: %v\n%s", err, out)
	}

	var got installResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("the harness printed something that is not its result: %v\n%s", err, out)
	}
	return got
}

func TestInstallChecksumFetchFollowsTheReleaseRedirect(t *testing.T) {
	body := "abc123  drift-linux-amd64\ndef456  drift-darwin-arm64\n"
	got := runInstallHarness(t, `[
		{"statusCode": 302, "headers": {"location": "https://objects.example.test/SHA256SUMS.txt"}},
		{"statusCode": 200, "body": `+jsonString(body)+`}
	]`)

	if !got.OK {
		t.Fatalf("downloadChecksums failed on a redirect, which is what every real install gets: %s", got.Error)
	}
	if len(got.Seen) != 2 {
		t.Fatalf("made %d requests, want 2 — the redirect was not followed", len(got.Seen))
	}
	if got.Seen[1] != "https://objects.example.test/SHA256SUMS.txt" {
		t.Errorf("second request went to %q, want the redirect target", got.Seen[1])
	}

	// And the checksums are parsed from the redirected response, so verification
	// can actually run rather than silently skipping.
	if got.Sums["drift-linux-amd64"] != "abc123" {
		t.Errorf("drift-linux-amd64 = %q, want abc123 — the body was not parsed", got.Sums["drift-linux-amd64"])
	}
	if got.Sums["drift-darwin-arm64"] != "def456" {
		t.Errorf("drift-darwin-arm64 = %q, want def456", got.Sums["drift-darwin-arm64"])
	}
}

// A redirect that never resolves has to end as an error. Following Location
// unconditionally turns a misconfigured release into a postinstall that hangs
// until npm's timeout, which reads as a hung install rather than a broken one.
func TestInstallChecksumFetchGivesUpOnARedirectLoop(t *testing.T) {
	loop := `{"statusCode": 302, "headers": {"location": "https://github.com/loop"}}`
	got := runInstallHarness(t, "["+loop+","+loop+","+loop+","+loop+","+loop+","+loop+","+loop+","+loop+"]")

	if got.OK {
		t.Fatal("downloadChecksums followed a redirect loop to completion")
	}
	if got.Error == "" {
		t.Error("the loop failed without saying why")
	}
}

// A non-redirect failure still has to fail, so the fix was not "accept anything".
func TestInstallChecksumFetchStillRejectsAnError(t *testing.T) {
	got := runInstallHarness(t, `[{"statusCode": 404}]`)

	if got.OK {
		t.Fatal("downloadChecksums accepted a 404")
	}
	if got.Error != "Checksums HTTP 404" {
		t.Errorf("error = %q, want it to name the status", got.Error)
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
