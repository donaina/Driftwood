package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

/* `download` follows GitHub's 302 to objects.githubusercontent.com. It used to
   resolve the redirect by closing the write stream it had already opened and
   calling itself, which left the redirecting request alive and still holding the
   path: never destroyed, its response body never resumed, and its 30-second timer
   still armed over a destPath it no longer owned.

   That timer is the defect. Fired while the recursive download was still running
   it failed an install that would have succeeded; fired after it finished, its
   reject did nothing — the promise had already resolved true — while the
   fs.unlink next to it ran anyway and deleted the file the other call had just
   written. install() then printed "Verifying SHA256 checksum..." and threw ENOENT
   reading a file that had existed a moment earlier, and fell back to a source
   build. Both failure modes were observed against the published package before
   this test existed, which is also why the assertions are about the file
   surviving rather than about the promise's value: the promise said true the
   whole time.

   The request that succeeds has the same defect in the same shape: thirty
   seconds is armed on it too, nothing clears it when the body lands, and a
   keep-alive socket that goes idle fires it at a file the download has already
   finished. Both are the one rule — a request owns destPath only while it is
   waiting on it — so both are asserted here.

   The harness drives the real download over a stubbed transport and then does
   deliberately what each request would have done on its own — fires its timer —
   so "the file is still there" is an assertion about ownership and not about
   luck. */

// downloadHarness replaces https.get with a queue of canned responses and counts
// the write streams opened on the destination, then calls the real download.
const downloadHarness = `
const https = require('https');
const fs = require('fs');

const script = process.argv[2];
const dest = process.argv[3];
const queue = JSON.parse(process.argv[4]);

const requests = [];
let streams = 0;

const origCreate = fs.createWriteStream;
fs.createWriteStream = function (p) {
  if (p === dest) streams++;
  return origCreate.apply(fs, arguments);
};

https.get = function (url, cb) {
  const entry = { url: url, resumed: false, timeoutMs: null, timer: null };
  requests.push(entry);
  const r = queue.shift();
  if (!r) throw new Error('unexpected extra request: ' + url);
  const res = {
    statusCode: r.statusCode,
    headers: r.headers || {},
    resume() { entry.resumed = true; },
    pipe(d) { if (r.body) d.write(r.body); d.end(); return d; },
    setEncoding() {},
    on() { return this; },
  };
  process.nextTick(() => cb(res));
  return {
    on() { return this; },
    setTimeout(ms, fn) { entry.timeoutMs = ms; entry.timer = fn; return this; },
    destroy() {},
  };
};

require(script).download('https://github.com/release/drift-darwin-x64', dest)
  .then(async () => {
    // Everything the request that got the redirect could still do to destPath,
    // done on purpose. A download that took ownership properly has nothing
    // left to fire here.
    requests.forEach(r => { if (typeof r.timer === 'function') r.timer(); });
    await new Promise(r => setTimeout(r, 200));
    const there = fs.existsSync(dest);
    console.log(JSON.stringify({
      ok: true,
      urls: requests.map(r => r.url),
      exists: there,
      bytes: there ? fs.readFileSync(dest).length : 0,
      body: there ? fs.readFileSync(dest, 'utf8') : '',
      streams: streams,
      resumed: requests.map(r => r.resumed),
      timeoutMs: requests.map(r => r.timeoutMs),
    }));
  })
  .catch(err => {
    const there = fs.existsSync(dest);
    console.log(JSON.stringify({
      ok: false,
      error: err.message,
      urls: requests.map(r => r.url),
      exists: there,
      streams: streams,
    }));
  });
`

type downloadResult struct {
	OK        bool     `json:"ok"`
	URLs      []string `json:"urls"`
	Exists    bool     `json:"exists"`
	Bytes     int      `json:"bytes"`
	Body      string   `json:"body"`
	Streams   int      `json:"streams"`
	Resumed   []bool   `json:"resumed"`
	TimeoutMs []int    `json:"timeoutMs"`
	Error     string   `json:"error"`
}

func runDownloadHarness(t *testing.T, responses string) (downloadResult, string) {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on PATH; the gate builds the dashboard with npm, so this only skips outside it")
	}

	dir := t.TempDir()
	harness := filepath.Join(dir, "download-harness.js")
	if err := os.WriteFile(harness, []byte(downloadHarness), 0o644); err != nil {
		t.Fatalf("writing the harness: %v", err)
	}
	install, err := filepath.Abs(filepath.Join("..", "bin", "install.js"))
	if err != nil {
		t.Fatalf("resolving bin/install.js: %v", err)
	}
	dest := filepath.Join(dir, "drift-bin")

	out, err := exec.Command(node, harness, install, dest, responses).Output()
	if err != nil {
		t.Fatalf("the harness exited non-zero: %v\n%s", err, out)
	}

	var got downloadResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("the harness printed something that is not its result: %v\n%s", err, out)
	}
	return got, dest
}

func TestInstallDownloadSurvivesTheRedirectItFollowed(t *testing.T) {
	const body = "a driftwood release binary, in spirit"
	got, dest := runDownloadHarness(t, `[
		{"statusCode": 302, "headers": {"location": "https://objects.example.test/drift-darwin-x64"}},
		{"statusCode": 200, "body": `+jsonString(body)+`}
	]`)

	if !got.OK {
		t.Fatalf("download failed on a redirect, which is what every real install gets: %s", got.Error)
	}
	if len(got.URLs) != 2 {
		t.Fatalf("made %d requests, want 2 — the redirect was not followed", len(got.URLs))
	}

	// The file the download wrote is still the file on disk, after the request
	// that got the redirect has done everything it could still do.
	if !got.Exists {
		t.Fatalf("the downloaded file is gone: the request that was redirected still "+
			"unlinked %s after handing it off", dest)
	}
	if got.Body != body {
		t.Errorf("downloaded body = %q, want %q", got.Body, body)
	}

	// One owner. Two streams over one path means both the redirecting call and
	// the redirect target opened a 'w' handle, so either could truncate the other.
	if got.Streams != 1 {
		t.Errorf("opened %d write streams on the destination, want 1", got.Streams)
	}

	// The redirect's body is not read, but the socket still has to be released.
	// The request that pipes its body into the file is consumed by the pipe
	// instead, so it is the first one and only the first one that resumes.
	if !got.Resumed[0] {
		t.Error("the redirect response was never resumed, so its socket is never freed")
	}

	// No request may be left holding a timer over a path it is finished with, and
	// that is not only about the redirect: an idle keep-alive socket fires its
	// timer thirty seconds after a *successful* download too, and the handler
	// behind it unlinks the file that was just completed.
	if len(got.TimeoutMs) != 2 {
		t.Fatalf("made %d requests, want 2", len(got.TimeoutMs))
	}
	for i, ms := range got.TimeoutMs {
		if ms != 0 {
			t.Errorf("request %d kept a %dms timeout armed over the destination after it was done",
				i+1, ms)
		}
	}
}

// A failure that is not a redirect still has to fail, and must not leave a
// half-written binary behind for the next install to find and trust.
func TestInstallDownloadStillRejectsAnError(t *testing.T) {
	got, _ := runDownloadHarness(t, `[{"statusCode": 404}]`)

	if got.OK {
		t.Fatal("download accepted a 404")
	}
	if got.Error != "HTTP status 404" {
		t.Errorf("error = %q, want it to name the status", got.Error)
	}
	if got.Exists {
		t.Error("a rejected download left a file on disk")
	}
}

func TestInstallDownloadGivesUpOnARedirectLoop(t *testing.T) {
	loop := `{"statusCode": 302, "headers": {"location": "https://github.com/loop"}}`
	got, _ := runDownloadHarness(t, "["+loop+","+loop+","+loop+","+loop+","+loop+","+loop+","+loop+","+loop+"]")

	if got.OK {
		t.Fatal("download followed a redirect loop to completion")
	}
	if got.Error != "Download redirect loop" {
		t.Errorf("error = %q, want it to name the loop", got.Error)
	}
}

// A 302 with no Location is a broken release, not a download that should sit
// there until the timeout.
func TestInstallDownloadRejectsARedirectWithoutALocation(t *testing.T) {
	got, _ := runDownloadHarness(t, `[{"statusCode": 302, "headers": {}}]`)

	if got.OK {
		t.Fatal("download accepted a redirect with no Location")
	}
	if got.Error != "Download redirect carried no Location" {
		t.Errorf("error = %q, want it to name the missing header", got.Error)
	}
}
