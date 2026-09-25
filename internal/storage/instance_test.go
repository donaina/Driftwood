package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstanceHintRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := InstanceInfo{
		ControlURL: "http://127.0.0.1:8787",
		PID:        4242,
		StartedAt:  time.Now().Truncate(time.Second),
	}
	if err := WriteInstance(want); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	got, ok := ReadInstance()
	if !ok {
		t.Fatal("ReadInstance found nothing after WriteInstance")
	}
	if got.ControlURL != want.ControlURL {
		t.Errorf("ControlURL = %q, want %q", got.ControlURL, want.ControlURL)
	}
	if got.PID != want.PID {
		t.Errorf("PID = %d, want %d", got.PID, want.PID)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}

	// It lives beside the store, not somewhere else: a hint in a different
	// directory from the state it describes would point an import at an
	// instance that is serving a different install's store.
	if filepath.Dir(instanceHintPath(t)) != PersistentDir() {
		t.Errorf("the hint is at %s, not in %s", instanceHintPath(t), PersistentDir())
	}
}

// The hint is written with the permissions of a file only its owner should
// read. It holds no secret today, but "what a local file may contain" is not a
// property to leave to whoever edits this next — the store beside it is 0600
// and this is written by the same code path.
func TestInstanceHintIsOwnerOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := WriteInstance(InstanceInfo{ControlURL: "http://127.0.0.1:8787", PID: 1}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	info, err := os.Stat(instanceHintPath(t))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("the hint is mode %04o, want 0600", perm)
	}
}

// ClearInstance removes the hint only while it still names the process that is
// leaving.
//
// The listener closes early in shutdown and the alert queue drains after it, so
// a replacement instance can start and record itself while the outgoing one is
// still finishing. Deleting unconditionally would then take away the address of
// the instance that is actually running, and the next import would write the
// store file behind a live proxy — the bug this whole mechanism exists to fix.
func TestClearInstanceLeavesAnotherProcessesHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	replacement := InstanceInfo{ControlURL: "http://127.0.0.1:9999", PID: 2222}
	if err := WriteInstance(replacement); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	ClearInstance(1111) // the process that is leaving

	got, ok := ReadInstance()
	if !ok {
		t.Fatal("ClearInstance removed a hint belonging to a different process")
	}
	if got.PID != replacement.PID {
		t.Errorf("the hint now names pid %d, want %d", got.PID, replacement.PID)
	}
}

func TestClearInstanceRemovesItsOwnHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := WriteInstance(InstanceInfo{ControlURL: "http://127.0.0.1:8787", PID: 4242}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	ClearInstance(4242)

	if _, ok := ReadInstance(); ok {
		t.Error("the hint survived ClearInstance for its own pid")
	}
}

// Everything that is not a usable hint reads as no hint.
//
// The caller has a fallback for "no instance", so telling the difference
// between absent, truncated and URL-less would only be a distinction it cannot
// act on — and a half-written file is what a crash mid-write leaves. The writes
// are atomic, so this is belt and braces; it is also the case a hand-edited
// file produces.
func TestReadInstanceTreatsUnusableHintsAsNone(t *testing.T) {
	cases := map[string]string{
		"absent":        "",
		"empty file":    "",
		"truncated":     `{"control_url": "http://127.0`,
		"wrong shape":   `["not", "an", "object"]`,
		"no url":        `{"pid": 7}`,
		"blank url":     `{"control_url": "   ", "pid": 7}`,
		"nonsense file": "\x00\x01\x02",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if content != "" {
				writeRawHint(t, content)
			}
			if info, ok := ReadInstance(); ok {
				t.Errorf("ReadInstance accepted a %s hint as %+v", name, info)
			}
		})
	}
}

func TestPersistentDirFollowsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := PersistentDir(), filepath.Join(home, ".driftwood"); got != want {
		t.Errorf("PersistentDir() = %q, want %q", got, want)
	}
}

// instanceHintPath is where the hint lands under the current $HOME. Derived
// here rather than exported from the package: the test is asserting the
// location, so a helper that the code under test also used would assert that
// the code agrees with itself.
func instanceHintPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(PersistentDir(), "instance.json")
}

func writeRawHint(t *testing.T, content string) {
	t.Helper()
	dir := PersistentDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(instanceHintPath(t), []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// A guard on the harness, in the same spirit as the one in cmd/drift: if the
// hint ever stops being resolved from $HOME, these tests would start writing
// into the developer's real ~/.driftwood with no visible symptom.
func TestInstanceHintDoesNotTouchTheRealHome(t *testing.T) {
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory to protect")
	}
	realHint := filepath.Join(realHome, ".driftwood", "instance.json")
	before, beforeErr := os.Stat(realHint)

	t.Setenv("HOME", t.TempDir())
	if err := WriteInstance(InstanceInfo{ControlURL: "http://127.0.0.1:8787", PID: os.Getpid()}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}
	ClearInstance(os.Getpid())

	after, afterErr := os.Stat(realHint)
	if beforeErr == nil && afterErr == nil && !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("%s was modified during the test", realHint)
	}
	if beforeErr != nil && afterErr == nil {
		t.Errorf("the test created %s in the real home directory", realHint)
	}
}

// The file has to be JSON an older or newer reader would both accept, since the
// CLI and the server are separate binaries that can be different builds. This
// pins the field names: renaming one silently breaks the pair, and the symptom
// is an import falling back to the file rather than an error.
func TestInstanceHintFieldNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := WriteInstance(InstanceInfo{ControlURL: "http://127.0.0.1:8787", PID: 7}); err != nil {
		t.Fatalf("WriteInstance: %v", err)
	}

	raw, err := os.ReadFile(instanceHintPath(t))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("the hint is not a JSON object: %v", err)
	}
	for _, want := range []string{"control_url", "pid", "started_at"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("the hint has no %q field: %s", want, raw)
		}
	}
}
