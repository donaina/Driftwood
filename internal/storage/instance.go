package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/* Where a running instance says so.

   `drift import` is a second process writing the same document the proxy
   writes, and the proxy's next save serialises its whole in-memory view — so
   the import was erased by the next thing the dashboard did, silently, with
   every request still answering 200. Measured: a contract imported into a
   running install was gone after one ordinary save.

   The fix is for the import to go through the running instance rather than
   around it, and this file is how it finds it. A hint rather than a fact: the
   reader confirms by asking the URL it names, so an instance killed without
   cleaning up costs one refused connection and nothing else. That is the
   property a lock file does not have without platform-specific code, and this
   package has none. */

// The route an instance answers on to confirm it is Driftwood is not named here
// on purpose. It is a control route, it is built from proxy.ControlPrefix in
// both the server that serves it and the command line that asks for it, and
// this package does not import the proxy. A third copy of the path on this side
// would be the one that drifts.
const instanceFileName = "instance.json"

// InstanceInfo is what a running instance writes down about itself.
type InstanceInfo struct {
	// ControlURL is the base every control route hangs off, with no trailing
	// slash. Written as the address a client on this machine should dial, which
	// is loopback even when the instance is bound to a wider interface.
	ControlURL string `json:"control_url"`

	// PID is what lets ClearInstance tell this process's hint from a later
	// instance's.
	PID int `json:"pid"`

	StartedAt time.Time `json:"started_at"`
}

// PersistentDir is the directory this install keeps its state in: the store,
// the config, and the instance hint.
//
// Exported because the import path has to look for a running instance before it
// decides whether to open a store at all, and opening one is not free of
// consequence — NewStore seeds the default project and persists it, so a CLI
// that opened a store merely to find a directory would write the file it was
// trying to avoid writing.
func PersistentDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".driftwood")
}

// WriteInstance records that this process is serving the control plane at
// controlURL.
//
// A failure is returned rather than fatal: the hint is what makes an import go
// through the API instead of round the back of it, and a read-only home
// directory is not a reason to refuse to serve.
func WriteInstance(info InstanceInfo) error {
	dir := PersistentDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, instanceFileName), data, 0600)
}

// ClearInstance removes the hint, but only while it still names the process
// that is leaving.
//
// Unconditional removal would be wrong in the case it is easiest to get wrong:
// a second instance started while the first was still shutting down owns the
// file by then, and deleting it on the way out would leave the next import
// looking at a hint that describes nothing.
func ClearInstance(pid int) {
	info, ok := ReadInstance()
	if !ok || info.PID != pid {
		return
	}
	_ = os.Remove(filepath.Join(PersistentDir(), instanceFileName))
}

// ReadInstance returns the hint a previous instance left, if there is one that
// can be read.
//
// Not existing, not parsing, and naming no URL are all the same answer — "no
// instance to ask" — because all three lead to the same behaviour, and a caller
// with a fallback does not need to tell them apart. The hint is unverified: it
// says where an instance was, not that one is still there.
func ReadInstance() (InstanceInfo, bool) {
	data, err := os.ReadFile(filepath.Join(PersistentDir(), instanceFileName))
	if err != nil {
		return InstanceInfo{}, false
	}
	var info InstanceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return InstanceInfo{}, false
	}
	// Trimmed, not merely compared to "": a hint holding spaces is a hand-edit
	// or a truncated write, and passing it on produces a request URL of " /…"
	// whose failure is reported as a protocol error rather than as the absent
	// hint it is.
	if strings.TrimSpace(info.ControlURL) == "" {
		return InstanceInfo{}, false
	}
	return info, true
}
