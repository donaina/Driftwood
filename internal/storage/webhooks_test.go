package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

func webhookConfig(kind, url string, enabled bool) types.WebhookConfig {
	return types.WebhookConfig{Kind: kind, URL: url, Enabled: enabled}
}

func TestSetAndGetWebhooks(t *testing.T) {
	driftwoodHome(t)
	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	project := store.ActiveProject()

	if got := store.GetWebhooks(project); len(got) != 0 {
		t.Errorf("a fresh store reported %d webhooks, want none", len(got))
	}

	for _, kind := range []string{types.WebhookSlack, types.WebhookDiscord} {
		if err := store.SetWebhook(project, webhookConfig(kind, "https://example.com/"+kind, true)); err != nil {
			t.Fatalf("SetWebhook(%s): %v", kind, err)
		}
	}

	got := store.GetWebhooks(project)
	if len(got) != 2 {
		t.Fatalf("got %d webhooks, want 2", len(got))
	}

	// WebhookKinds order, not insertion order and not map order: the dashboard
	// renders four cards and they must not reorder themselves between loads.
	if got[0].Kind != types.WebhookSlack || got[1].Kind != types.WebhookDiscord {
		t.Errorf("kinds came back as %q, %q; want slack then discord", got[0].Kind, got[1].Kind)
	}

	one, ok := store.GetWebhook(project, types.WebhookTeams)
	if ok {
		t.Error("an unconfigured kind reported as present")
	}
	if one.URL != "" {
		t.Errorf("unconfigured kind carried a URL: %q", one.URL)
	}
}

func TestSetWebhookRefusesUnknownKind(t *testing.T) {
	driftwoodHome(t)
	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.SetWebhook(store.ActiveProject(), webhookConfig("carrier-pigeon", "https://example.com", true)); err == nil {
		t.Error("an unknown kind was stored; a config nothing renders is a setting with no effect")
	}
}

func TestSetWebhookRefusesUnknownProject(t *testing.T) {
	driftwoodHome(t)
	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.SetWebhook("nope", webhookConfig(types.WebhookSlack, "https://example.com", true)); err == nil {
		t.Error("a webhook was stored against a project that does not exist")
	}
}

// A patch that sets one field must not disturb the others. The server does the
// overlay, but this pins the property it relies on: UpdateAt is the store's, and
// the three fields survive a round trip through the file.
func TestWebhookSurvivesRestart(t *testing.T) {
	dir := driftwoodHome(t)

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	project := store.ActiveProject()

	secret := "s3cr3t-signing-key"
	before := time.Now().Add(-time.Minute)
	if err := store.SetWebhook(project, types.WebhookConfig{
		Kind: types.WebhookGeneric, URL: "https://example.com/hook", Enabled: true, Secret: secret,
	}); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	// A second store over the same directory, which is what a restart is.
	reopened, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}

	got, ok := reopened.GetWebhook(project, types.WebhookGeneric)
	if !ok {
		t.Fatal("the configured channel did not survive the restart")
	}
	if got.URL != "https://example.com/hook" {
		t.Errorf("URL = %q", got.URL)
	}
	if !got.Enabled {
		t.Error("the channel came back disabled")
	}
	if got.Secret != secret {
		t.Errorf("secret did not survive: %q", got.Secret)
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt = %v, want a time the store set at write", got.UpdatedAt)
	}

	if _, err := os.Stat(filepath.Join(dir, "baselines.json")); err != nil {
		t.Fatalf("no store file was written: %v", err)
	}
}

func TestDeleteWebhook(t *testing.T) {
	driftwoodHome(t)
	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	project := store.ActiveProject()

	if err := store.SetWebhook(project, webhookConfig(types.WebhookSlack, "https://example.com/slack", true)); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	if err := store.DeleteWebhook(project, types.WebhookSlack); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if _, ok := store.GetWebhook(project, types.WebhookSlack); ok {
		t.Error("the channel was still present after being deleted")
	}

	// Deleting what is not there is not a failure: the caller asked for it to be
	// gone, and it is.
	if err := store.DeleteWebhook(project, types.WebhookTeams); err != nil {
		t.Errorf("deleting an unconfigured kind reported an error: %v", err)
	}
}

func TestDeleteProjectRemovesItsWebhooks(t *testing.T) {
	driftwoodHome(t)
	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	first := store.ActiveProject()
	second, err := store.CreateProject("Second")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	for _, project := range []string{first, second.ID} {
		if err := store.SetWebhook(project, webhookConfig(types.WebhookSlack, "https://example.com/"+project, true)); err != nil {
			t.Fatalf("SetWebhook(%s): %v", project, err)
		}
	}

	if err := store.DeleteProject(second.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// The surviving project's config must be untouched — the point of the
	// per-project dimension is that one client's channels are not another's.
	kept, ok := store.GetWebhook(first, types.WebhookSlack)
	if !ok || kept.URL != "https://example.com/"+first {
		t.Errorf("the surviving project's channel was disturbed: %+v (ok=%v)", kept, ok)
	}
	if got := store.GetWebhooks(second.ID); len(got) != 0 {
		t.Errorf("the deleted project still reports %d webhooks", len(got))
	}
	if _, ok := store.GetWebhook(second.ID, types.WebhookSlack); ok {
		t.Error("the deleted project's channel is still reachable")
	}
}

// An unchanged store with webhooks configured must write the same bytes twice.
// This is the property stateForPersistLocked exists to preserve, and the one a
// new map is most likely to break: a document that reshuffles itself on every
// save makes every write look like a change, and makes the file useless to diff.
func TestWebhookPersistIsByteIdentical(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	project := store.ActiveProject()

	for _, kind := range types.WebhookKinds {
		if err := store.SetWebhook(project, webhookConfig(kind, "https://example.com/"+kind, true)); err != nil {
			t.Fatalf("SetWebhook(%s): %v", kind, err)
		}
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}

	// Reopen and save again with nothing changed. NewStore does not write, so
	// this second save is the one being compared.
	reopened, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if err := reopened.DeleteWebhook(project, "not-a-kind"); err != nil {
		t.Fatalf("triggering a save: %v", err)
	}

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store back: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("an unchanged save rewrote the document.\n--- before ---\n%s\n--- after ---\n%s", first, second)
	}
}

// A store written before webhooks existed must load, keep its endpoints, and not
// be rewritten just because this build can hold a field the document lacks.
func TestV2StoreLoadsWithNoWebhooks(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	// Built from the real type rather than hand-typed, the way writeV1 builds its
	// fixture: a hand-written approximation is a fixture for the shape someone
	// remembered, not the shape the old build actually wrote. Version 2 with
	// Webhooks omitted by omitempty is exactly what a v2 build produced.
	now := time.Now().Truncate(time.Second)
	doc := persistedState{
		Version:  2,
		Active:   "acme",
		Projects: []types.Project{{ID: "acme", Name: "Acme", CreatedAt: now}},
		Histories: map[string]map[string]*types.EndpointHistory{
			"acme": {"GET:/api/users": v1History("GET", "/api/users", `{"id": 7}`, now)},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("building the v2 fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing the v2 fixture: %v", err)
	}

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("a v2 store was refused: %v", err)
	}
	if got := store.ActiveProject(); got != "acme" {
		t.Errorf("active project = %q, want acme", got)
	}
	if _, ok := store.GetBaseline("acme", "GET", "/api/users"); !ok {
		t.Error("the v2 document's endpoint was not loaded")
	}
	if got := store.GetWebhooks("acme"); len(got) != 0 {
		t.Errorf("a v2 document reported %d webhooks", len(got))
	}

	// Reading it must not have rewritten it. The version is only stamped on the
	// next natural save; see TestV2StoreIsReadAndNotRewritten.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the store back: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Error("loading a v2 store rewrote it")
	}
	if _, err := os.Stat(path + ".v1"); !os.IsNotExist(err) {
		t.Error("reading a v2 store produced a v1 migration copy")
	}
}

// A document from a future build is refused, and the error names both numbers.
// This is the data-loss guard: such a document may hold a field this build would
// drop on its next save.
func TestFutureStoreVersionIsRefused(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	doc := persistedState{Version: storeVersion + 1, Active: defaultProjectID}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("building the fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	store, err := NewStore("http://localhost:3000", "8787")
	if err == nil {
		t.Fatal("a store from a future version was accepted")
	}
	if store == nil {
		t.Fatal("NewStore returned no store alongside its error; it must always be usable")
	}

	// Moved aside, so the next save cannot silently overwrite it.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("the refused document was left where the next write would overwrite it")
	}
}

// The webhook secret is the reason the store file's permissions matter, so this
// asserts them rather than assuming the 0600 that persistLocked passes.
func TestWebhookSecretIsWrittenToA0600Store(t *testing.T) {
	dir := driftwoodHome(t)
	path := filepath.Join(dir, "baselines.json")

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.SetWebhook(store.ActiveProject(), types.WebhookConfig{
		Kind: types.WebhookGeneric, URL: "https://example.com/hook", Enabled: true, Secret: "signing-key",
	}); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("store file mode = %o, want 600 — it holds a signing secret", perm)
	}
}

// AllowPrivate is the deliverer's permission to dial a private address, and it
// has to survive a restart like any other part of the config.
//
// This is not a symmetry check. The deliverer runs in a background worker with no
// request to look at, so the flag on disk is the *only* thing that lets a
// loopback receiver keep working after a restart. Losing it would turn every
// local channel into one the dashboard shows as enabled and that never delivers
// anything — with the failure recorded against the operator's own receiver.
func TestWebhookAllowPrivateSurvivesRestart(t *testing.T) {
	driftwoodHome(t)

	store, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	project := store.ActiveProject()

	if err := store.SetWebhook(project, types.WebhookConfig{
		Kind: types.WebhookGeneric, URL: "http://127.0.0.1:9099/hook",
		Enabled: true, AllowPrivate: true,
	}); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	// The other channel, with the flag off, so the assertion is that the value
	// round-trips rather than that the field defaults to true on load.
	if err := store.SetWebhook(project, types.WebhookConfig{
		Kind: types.WebhookSlack, URL: "https://hooks.slack.com/services/T/B/X", Enabled: true,
	}); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	reopened, err := NewStore("http://localhost:3000", "8787")
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}

	permitted, ok := reopened.GetWebhook(project, types.WebhookGeneric)
	if !ok {
		t.Fatal("the local channel did not survive the restart")
	}
	if !permitted.AllowPrivate {
		t.Error("the permission to dial a private address did not survive the restart, " +
			"so every delivery to this channel will now be refused")
	}

	guarded, _ := reopened.GetWebhook(project, types.WebhookSlack)
	if guarded.AllowPrivate {
		t.Error("a public URL came back carrying the private-address permission")
	}
}
