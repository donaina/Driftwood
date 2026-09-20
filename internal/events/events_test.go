package events

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestScopeMatches pins the two empty values apart.
//
// They look alike and mean different things, which is exactly the kind of pair
// that gets collapsed by a later simplification: an event with no project is
// install-level and belongs to everyone, while a subscriber with no scope asked
// for everything. Collapsing them would either stop install-level events
// reaching scoped subscribers — the dashboard would never learn the project list
// changed — or deliver one project's traffic to every subscriber, which is the
// isolation this exists to provide.
func TestScopeMatches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		scope     string
		projectID string
		want      bool
	}{
		{"same project reaches its subscriber", "alpha", "alpha", true},
		{"another project does not", "alpha", "beta", false},
		{"install-level reaches a scoped subscriber", "alpha", "", true},
		{"install-level reaches an unscoped subscriber", "", "", true},
		{"any project reaches an unscoped subscriber", "", "beta", true},
	} {
		if got := scopeMatches(tc.scope, tc.projectID); got != tc.want {
			t.Errorf("%s: scopeMatches(%q, %q) = %v, want %v", tc.name, tc.scope, tc.projectID, got, tc.want)
		}
	}
}

// sseClient is one open event stream, with the frames it has received so far on
// a channel.
type sseClient struct {
	frames chan map[string]interface{}
	close  func()
}

// subscribe opens a stream through the real handler and returns once the stream
// is ready to receive.
//
// Waiting for the initial ping is what makes these tests deterministic rather
// than occasionally passing: the handler registers the subscriber before writing
// the ping, so a ping read back means a publish from this point on will be
// fanned out. Publishing straight after the GET races registration, and the
// failure looks like a dropped event rather than a test bug.
func subscribe(t *testing.T, hub *Hub, projectID string) *sseClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.SSEHandler(w, r, projectID)
	}))

	resp, err := http.Get(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("opening the stream: %v", err)
	}

	frames := make(chan map[string]interface{}, 32)
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		first := true
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			if first {
				// The ping. Registration has happened; from here events arrive.
				close(ready)
				first = false
				continue
			}
			var frame map[string]interface{}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
				continue
			}
			frames <- frame
		}
	}()

	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		srv.Close()
		t.Fatal("the stream never sent its opening ping")
	}

	return &sseClient{
		frames: frames,
		close: func() {
			_ = resp.Body.Close()
			srv.Close()
		},
	}
}

// next returns the next frame, or nil if none arrives in time.
func (c *sseClient) next(t *testing.T) map[string]interface{} {
	t.Helper()
	select {
	case f := <-c.frames:
		return f
	case <-time.After(500 * time.Millisecond):
		return nil
	}
}

// TestOneProjectsEventsDoNotReachAnothersSubscriber is the test SSE scoping
// exists for.
//
// Unfiltered, a dashboard switched to one client still receives every other
// client's live alerts — isolation that would be cosmetic precisely where the
// dashboard is most alive, and would show one client's endpoint names and
// breaking changes in another client's session.
func TestOneProjectsEventsDoNotReachAnothersSubscriber(t *testing.T) {
	hub := NewHub()
	alpha := subscribe(t, hub, "alpha")
	defer alpha.close()
	beta := subscribe(t, hub, "beta")
	defer beta.close()

	hub.Publish("beta", "alert", map[string]interface{}{"endpoint": "GET /orders"})
	hub.Publish("alpha", "alert", map[string]interface{}{"endpoint": "GET /users"})

	// The first frame on alpha's stream must be alpha's own event. If beta's
	// arrived it would be here first, since it was published first.
	frame := alpha.next(t)
	if frame == nil {
		t.Fatal("alpha's subscriber received nothing")
	}
	if frame["type"] != "alert" {
		t.Fatalf("alpha received a %v frame, want alert", frame["type"])
	}
	data, _ := frame["data"].(map[string]interface{})
	if data["endpoint"] != "GET /users" {
		t.Errorf("alpha received %v, want its own endpoint — another project's event reached this subscriber", data["endpoint"])
	}

	// And nothing follows it. This is the half that a "did the right event
	// arrive" assertion alone would miss: alpha getting its own event is also
	// consistent with alpha getting beta's afterwards.
	if extra := alpha.next(t); extra != nil {
		t.Errorf("alpha received a second frame %v; the only other event published was beta's", extra)
	}
}

// TestInstallLevelEventsReachEverySubscriber covers the other direction: an
// event belonging to no project is news to all of them.
func TestInstallLevelEventsReachEverySubscriber(t *testing.T) {
	hub := NewHub()
	alpha := subscribe(t, hub, "alpha")
	defer alpha.close()
	beta := subscribe(t, hub, "beta")
	defer beta.close()

	hub.Publish("", "projects_updated", map[string]interface{}{"active": "beta"})

	for name, c := range map[string]*sseClient{"alpha": alpha, "beta": beta} {
		frame := c.next(t)
		if frame == nil {
			t.Errorf("%s's subscriber received nothing; an install-level event should reach every scope", name)
			continue
		}
		if frame["type"] != "projects_updated" {
			t.Errorf("%s received a %v frame, want projects_updated", name, frame["type"])
		}
	}
}

// TestAnUnscopedSubscriberReceivesEverything documents the subscription that
// asks for no filter, which is what a test harness or a future "all projects"
// view uses.
func TestAnUnscopedSubscriberReceivesEverything(t *testing.T) {
	hub := NewHub()
	all := subscribe(t, hub, "")
	defer all.close()

	hub.Publish("alpha", "traffic", map[string]interface{}{"path": "/users"})
	hub.Publish("beta", "traffic", map[string]interface{}{"path": "/orders"})

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		frame := all.next(t)
		if frame == nil {
			break
		}
		data, _ := frame["data"].(map[string]interface{})
		seen[data["path"].(string)] = true
	}
	if !seen["/users"] || !seen["/orders"] {
		t.Errorf("an unscoped subscriber saw %v, want both projects' events", seen)
	}
}
