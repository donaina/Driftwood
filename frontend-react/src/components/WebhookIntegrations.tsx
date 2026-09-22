import React, { useCallback, useEffect, useState } from 'react';
import {
  Button,
  EmptyState,
  Field,
  Input,
  Panel,
  PanelTitle,
  RefreshIcon,
  SkeletonRows,
  ViewHeader,
  toast,
} from './ui';

/* Where this project's alerts are delivered.

   Every control on this screen is backed by a route that exists — GET and POST
   /api/webhooks, POST /api/webhooks/test and GET /api/webhooks/deliveries — and
   the two that can fail say so on the card that failed rather than replacing the
   view.

   This file used to be the counter-example the nav entry for it was withheld
   over. Save toasted "Configuration saved for slack with URL: …" over an empty
   function body, reading the URL back out of the DOM by element id; Test toasted
   "Test webhook sent to slack" without sending anything; three Alert Types
   checkboxes were read by nothing; and the header carried a disabled "Add
   Webhook" button whose tooltip said webhooks "are configured in the Driftwood
   proxy config file", which is not true of any field in types.ProxyConfig. Four
   controls reporting four pieces of work that never happened is the class of lie
   the last three releases were about removing.

   Two errors, not one. `error` is the load failing, and it replaces the view
   because there is nothing to show. `actionError` is one card's save or test
   failing, and it is rendered on that card: a save that did not take must not
   hide the configuration the operator was reading in order to say so. Same split
   as HistoryView. */

type Channel = {
  id: string;
  name: string;
  description: string;
  setupSteps: string[];
};

/* Four channels, and each one is an HTTP POST to a URL the operator supplies —
   they differ only in the shape of the body. Email is deliberately not among
   them: it is a different transport, `net/smtp` is frozen upstream, and nothing
   in the API could carry it. */
const CHANNELS: Channel[] = [
  {
    id: 'slack',
    name: 'Slack',
    description: 'Post drift alerts into a Slack channel.',
    setupSteps: [
      'Create a Slack app in your workspace and enable Incoming Webhooks.',
      'Add the webhook to the channel you want alerts in, and copy its URL.',
      'Paste the URL below and save.',
    ],
  },
  {
    id: 'teams',
    name: 'Microsoft Teams',
    description: 'Post drift alerts into a Teams channel.',
    setupSteps: [
      /* Office 365 connectors are retired, so "configure an Incoming Webhook
         connector in Teams" — what this card used to say — is not a thing an
         operator can do any more. A Power Automate workflow is the current path
         to a channel webhook, and it is the URL this expects. */
      'Create a Power Automate workflow in Teams that posts to your channel.',
      'Copy the HTTP POST URL the workflow gives you.',
      'Paste the URL below and save.',
    ],
  },
  {
    id: 'discord',
    name: 'Discord',
    description: 'Post drift alerts into a Discord channel.',
    setupSteps: [
      'Open your Discord channel settings and create a webhook.',
      'Copy the webhook URL.',
      'Paste the URL below and save.',
    ],
  },
  {
    id: 'generic',
    name: 'Generic webhook',
    description: 'POST the alert as JSON to an endpoint you run.',
    setupSteps: [
      'Run an endpoint that accepts a JSON POST and answers 2xx.',
      'Copy its URL.',
      'Paste the URL below and save.',
    ],
  },
];

/* GET /api/webhooks. `has_secret` rather than the secret: no route in the
   product returns one, so there is nothing here to display and no reveal
   affordance to build — an operator who has lost a secret rotates it. */
type WebhookView = {
  kind: string;
  url: string;
  enabled: boolean;
  has_secret: boolean;
  updated_at: string;
};

/* POST /api/webhooks/test, which is synchronous and reports what the receiver
   did. `status_code` is absent when the request never reached one. */
type TestResult = {
  ok: boolean;
  status_code?: number;
  latency_ms: number;
  error?: string;
};

/* GET /api/webhooks/deliveries. A Record carries our status code and our own
   error text and never a byte the receiver sent — see internal/webhook.Record. */
type DeliveryRecord = {
  id: string;
  endpoint: string;
  kind: string;
  status: string;
  attempts: number;
  status_code?: number;
  error?: string;
  detected_at: string;
  completed_at: string;
};

/** What the operator has typed, before it is saved. */
type Draft = { url: string; enabled: boolean };

const BASE = '/_driftwood/api/webhooks';

/** Only the ten newest are shown; the route returns fifty. */
const RECORDS_SHOWN = 10;

/* Severity tokens on a delivery outcome.

   DESIGN.md bans decorative use of these hues: healthy/amber/red mean contract
   state and nothing else. A delivery that did not arrive is the one neighbouring
   sense the vocabulary fits — it is a thing that is broken, not a thing that
   merely has a status — and the row says outright what it is describing, so the
   colour is not being asked to carry the meaning on its own. */
const STATUS_TONE: Record<string, string> = {
  delivered: 'text-accent-healthy',
  failed: 'text-accent-breaking',
  dropped: 'text-accent-warning',
};

/* The routes answer a refusal with a sentence written for the operator — "a
   channel needs a URL before it can be enabled", "unknown webhook kind" — and
   rendering the status alone would throw that sentence away and leave the card
   saying "400". */
async function failure(res: Response): Promise<string> {
  const body = (await res.text()).trim();
  return body ? `${res.status} ${body}` : `HTTP ${res.status}`;
}

/* What the receiver actually did, told apart by which fields came back: a 2xx
   carries the latency we measured, a refusal carries the vendor's own reply
   (which the route reads from the response body, and which no delivery record
   ever stores), and a transport error carries no status code at all because
   nothing answered. */
function describeTest(name: string, r: TestResult): string {
  if (r.ok) {
    return `Reached ${name} (${r.status_code} in ${r.latency_ms}ms)`;
  }
  if (r.status_code) {
    return `${name} refused the test message: ${r.status_code} ${r.error ?? ''}`.trim();
  }
  return `Could not reach the URL: ${r.error ?? 'the request failed without saying why'}`;
}

const WebhookIntegrations: React.FC = () => {
  const [configs, setConfigs] = useState<Record<string, WebhookView>>({});
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [records, setRecords] = useState<DeliveryRecord[]>([]);
  const [tests, setTests] = useState<Record<string, TestResult>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [testing, setTesting] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [configsRes, recordsRes] = await Promise.all([
        fetch(BASE),
        fetch(`${BASE}/deliveries`),
      ]);
      if (!configsRes.ok) throw new Error(await failure(configsRes));
      if (!recordsRes.ok) throw new Error(await failure(recordsRes));

      const list: WebhookView[] = await configsRes.json();
      const log: DeliveryRecord[] = await recordsRes.json();

      const byKind: Record<string, WebhookView> = {};
      for (const cfg of list) byKind[cfg.kind] = cfg;

      setConfigs(byKind);
      /* A channel with no saved config gets a draft too, so every card is
         controlled from the first render rather than flipping from uncontrolled
         to controlled when it is first saved. */
      setDrafts(
        Object.fromEntries(
          CHANNELS.map((channel) => [
            channel.id,
            { url: byKind[channel.id]?.url ?? '', enabled: byKind[channel.id]?.enabled ?? false },
          ])
        )
      );
      setRecords(log);
    } catch (err) {
      console.error(err);
      setError(`Could not load alert delivery settings: ${err}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const setDraft = (kind: string, next: Draft) => {
    setDrafts((prev) => ({ ...prev, [kind]: next }));
  };

  const clearActionError = (kind: string) => {
    setActionError((prev) => ({ ...prev, [kind]: '' }));
  };

  const save = async (channel: Channel) => {
    const draft = drafts[channel.id] ?? { url: '', enabled: false };
    setSaving(channel.id);
    clearActionError(channel.id);
    try {
      const res = await fetch(BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          kind: channel.id,
          url: draft.url.trim(),
          enabled: draft.enabled,
        }),
      });
      if (!res.ok) throw new Error(await failure(res));

      const list: WebhookView[] = await res.json();
      const byKind: Record<string, WebhookView> = {};
      for (const cfg of list) byKind[cfg.kind] = cfg;

      setConfigs(byKind);
      // Re-seed this card from what was stored, so the field shows the URL the
      // server kept rather than the one that was typed into it.
      setDraft(channel.id, {
        url: byKind[channel.id]?.url ?? '',
        enabled: byKind[channel.id]?.enabled ?? false,
      });

      const saved = byKind[channel.id];
      toast(
        `${channel.name} saved`,
        saved?.enabled
          ? `Driftwood will post alerts to ${saved.url}`
          : 'The URL is saved, but the channel is off — no alerts will be sent to it.'
      );
    } catch (err) {
      console.error(err);
      setActionError((prev) => ({ ...prev, [channel.id]: `Could not save ${channel.name}: ${err}` }));
    } finally {
      setSaving(null);
    }
  };

  const test = async (channel: Channel) => {
    setTesting(channel.id);
    clearActionError(channel.id);
    // Drop the previous result rather than leaving it under a spinner: a stale
    // "Reached Slack (200 in 142ms)" beside a button that is working again is
    // the same lie in a slower costume.
    setTests((prev) => {
      const next = { ...prev };
      delete next[channel.id];
      return next;
    });
    try {
      const res = await fetch(`${BASE}/test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: channel.id }),
      });
      if (!res.ok) throw new Error(await failure(res));

      const result: TestResult = await res.json();
      setTests((prev) => ({ ...prev, [channel.id]: result }));
      toast(`Test sent to ${channel.name}`, describeTest(channel.name, result));
    } catch (err) {
      console.error(err);
      setActionError((prev) => ({
        ...prev,
        [channel.id]: `Could not send a test to ${channel.name}: ${err}`,
      }));
    } finally {
      setTesting(null);
    }
  };

  const refresh = (
    <Button variant="primary" onClick={load}>
      <RefreshIcon />
      Refresh
    </Button>
  );

  if (loading) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Alert Delivery" align="center" action={refresh} />
        <SkeletonRows rows={4} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Alert Delivery" align="center" action={refresh} />
        <Panel tone="error" className="text-center text-accent-breaking" role="alert">
          {error}
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ViewHeader title="Alert Delivery" action={refresh} />

      {CHANNELS.map((channel) => {
        const cfg = configs[channel.id];
        const draft = drafts[channel.id] ?? { url: '', enabled: false };
        const result = tests[channel.id];
        const err = actionError[channel.id];

        /* Test sends to the address the server has stored, not to what is in
           the box. Unsaved edits therefore make it meaningless, and a Test that
           quietly exercised the previous URL would be a fifth control reporting
           work it did not do. */
        const dirty =
          draft.url.trim() !== (cfg?.url ?? '') || draft.enabled !== (cfg?.enabled ?? false);
        const saved = (cfg?.url ?? '') !== '';
        const testDisabled = saving !== null || testing !== null || dirty || !saved;
        const testTitle = !saved
          ? 'Save a URL for this channel before testing it.'
          : dirty
            ? 'Save first — a test is sent to the stored URL, not to what is typed in the box.'
            : undefined;

        return (
          <Panel key={channel.id}>
            <div className="flex items-start justify-between gap-4 max-[479px]:flex-col max-[479px]:items-start">
              <div>
                <h3 className="text-xl font-semibold text-text-main flex items-center gap-2">
                  {/* Neutral by construction. This badge used to be drawn in
                      accent-info, a severity hue, to label a product name. */}
                  <span
                    className="w-8 h-8 flex items-center justify-center rounded-lg border border-border-color bg-bg-hover text-text-muted text-lg font-semibold"
                    aria-hidden="true"
                  >
                    {channel.name.charAt(0)}
                  </span>
                  {channel.name}
                </h3>
                <p className="mt-2 text-text-muted text-base">{channel.description}</p>
              </div>

              {/* A switch, not a pressed button: the label is a fixed word and
                  aria-checked carries the state, which is the shape that reads
                  correctly to a screen reader. Generic HTML has no <switch>
                  element, which is why this is a button wearing the role.

                  accent-primary, not accent-healthy: this is an interactive
                  control, and a toggle drawn in the healthy hue would be saying
                  the endpoint's contract still matches its baseline.

                  Held shut while this card is saving, because the save's own
                  response re-seeds the draft — a toggle pressed mid-flight would
                  be silently reverted by the answer to the save that preceded
                  it. */}
              <Button
                size="sm"
                role="switch"
                variant={draft.enabled ? 'primary' : 'secondary'}
                aria-checked={draft.enabled}
                disabled={saving === channel.id}
                onClick={() => setDraft(channel.id, { ...draft, enabled: !draft.enabled })}
              >
                Enabled
              </Button>
            </div>

            <div className="mt-4 mb-4">
              <PanelTitle level={4} size="base" className="mb-2">
                Setup Steps
              </PanelTitle>
              <ol className="list-decimal list-inside space-y-1 text-sm text-text-muted">
                {channel.setupSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>

            <div className="mb-4 space-y-4">
              <Field label="Webhook URL">
                <Input
                  type="url"
                  value={draft.url}
                  placeholder="https://…"
                  onChange={(e) => setDraft(channel.id, { ...draft, url: e.target.value })}
                />
              </Field>

              {/* Three checkboxes stood here — Breaking Changes, Warnings and
                  Informational — and nothing in the product read them. Two of
                  the three claims were false and the third was impossible:
                  SeverityInfo deltas are carried inside an alert and never raise
                  one of their own, so no setting could have delivered them. A
                  sentence in the slot is the honest version of a control that
                  cannot be honoured yet. */}
              <Field label="Alert Types">
                <p className="text-sm text-text-muted">
                  Every alert this project raises is delivered here — breaking changes and
                  warnings. Informational deltas ride inside an alert rather than raising one,
                  and filtering is not configurable yet.
                </p>
              </Field>
            </div>

            <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4">
              <Button
                size="md"
                className="flex-1 sm:w-auto"
                onClick={() => test(channel)}
                disabled={testDisabled}
                busy={testing === channel.id}
                title={testTitle}
              >
                Send Test
              </Button>
              <Button
                size="md"
                variant="primary"
                className="flex-1 sm:w-auto"
                onClick={() => save(channel)}
                busy={saving === channel.id}
              >
                Save
              </Button>
            </div>

            {err && (
              <Panel pad="sm" tone="error" className="mt-4 text-accent-breaking" role="alert">
                {err}
              </Panel>
            )}

            {/* What the receiver did, kept on the page rather than only in a
                toast that is gone in five seconds — this is the one place an
                operator finds out that a vendor rejected our body, and the
                vendor's own words are in it. */}
            {result && (
              <p
                className={`mt-4 text-sm ${
                  result.ok ? 'text-accent-healthy' : 'text-accent-breaking'
                }`}
              >
                {describeTest(channel.name, result)}
              </p>
            )}
          </Panel>
        );
      })}

      {records.length === 0 ? (
        <EmptyState
          title="Nothing delivered yet"
          body={
            <>
              A record appears here each time Driftwood raises an alert and tries to send it.
              Alerts come from drift the proxy observes, so this fills in as requests pass
              through it rather than from anything you do on this screen.
            </>
          }
        />
      ) : (
        <Panel>
          <PanelTitle className="mb-2">Delivery records</PanelTitle>
          <p className="text-sm text-text-muted mb-4">
            {records.length > RECORDS_SHOWN
              ? `The ${RECORDS_SHOWN} most recent of ${records.length}. `
              : `${records.length} record${records.length === 1 ? '' : 's'}, newest first. `}
            Records hold the status Driftwood got back and its own error text — never
            anything the receiver sent.
          </p>
          <ul className="space-y-3">
            {records.slice(0, RECORDS_SHOWN).map((record) => (
              <li
                key={record.id}
                className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm border-b border-border-color pb-3 last:border-b-0 last:pb-0"
              >
                <span className={`font-semibold ${STATUS_TONE[record.status] ?? 'text-text-main'}`}>
                  {record.status}
                </span>
                <span className="font-mono text-text-main">{record.endpoint || 'no endpoint'}</span>
                <span className="text-text-muted">{record.kind}</span>
                <span className="text-text-muted">
                  {record.attempts} {record.attempts === 1 ? 'attempt' : 'attempts'}
                </span>
                {record.status_code ? (
                  <span className="text-text-muted">HTTP {record.status_code}</span>
                ) : null}
                <span className="text-text-muted">
                  {new Date(record.completed_at || record.detected_at).toLocaleString()}
                </span>
                {record.error && <span className="text-text-muted w-full">{record.error}</span>}
              </li>
            ))}
          </ul>
        </Panel>
      )}
    </div>
  );
};

export default WebhookIntegrations;
