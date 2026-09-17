import React, { useCallback, useEffect, useRef, useState } from 'react';
import { GITHUB_URL } from '../components/Nav';
import { Badge, LinkButton, Shell } from '../components/ui';
import {
  CONTROL_PREFIX,
  MOCK_PATH,
  type AlertEvent,
  type CapturedTraffic,
  type ContractDiff,
  type DiffDelta,
  type EventMessage,
  type MockMode,
  openStream,
  probe,
  sendMockRequest,
  setMockMode,
} from '../lib/driftwood';

/* The try-it-out page.
 *
 * It drives a real Driftwood instance — the one serving this page — through a
 * real breaking change, and shows the diff the server computed. Nothing on this
 * page is staged: the deltas come off the event stream as the proxy publishes
 * them, and if there is no instance the page says so instead of showing a
 * canned result. A demo that falls back to pre-recorded output is worse than no
 * demo, because the reader takes away a belief about the product that nothing
 * they did actually established.
 *
 * The one thing added for presentation is ordering. Left alone, the page would
 * be a mode dropdown and a request button; instead it runs the sequence in the
 * order the product does — healthy response, then a broken one, then what
 * Driftwood did about it — because the interesting part is not that a diff
 * happened but that the first request was fine and the second one was not.
 */

const BREAK_MODES: Array<{ mode: MockMode; label: string; blurb: string }> = [
  { mode: 'TYPE_BREAK', label: 'Type changed', blurb: 'id was an integer, now a string' },
  { mode: 'MISSING_FIELD', label: 'Field removed', blurb: 'email is gone entirely' },
  { mode: 'NULL_BREAK', label: 'Non-null returned null', blurb: 'email is present but null' },
  { mode: 'ADDED_FIELD', label: 'Field added', blurb: 'a new key appears — not a break' },
];

type RunResult = {
  mode: MockMode;
  status: number;
  body: string;
  traffic: CapturedTraffic | null;
};

type Phase = 'probing' | 'offline' | 'ready';

const prettyBody = (raw: string): string => {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
};

/* Severity is a string on the wire, but only four values are meaningful and
   every one of them needs a colour. An unknown value falls through to neutral
   rather than being coloured as if it were understood. */
const severityTone = (severity: string): string => {
  switch (severity) {
    case 'BREAKING':
      return 'text-accent-breaking border-accent-breaking';
    case 'WARNING':
      return 'text-accent-warning border-accent-warning';
    case 'INFO':
      return 'text-accent-info border-accent-info';
    default:
      return 'text-text-muted border-border-strong';
  }
};

const statusTone = (status: string): string => {
  switch (status) {
    case 'BREAKING':
      return 'text-accent-breaking';
    case 'WARNING':
      return 'text-accent-warning';
    case 'MATCH':
      return 'text-accent-healthy';
    default:
      return 'text-text-muted';
  }
};

const DeltaRow: React.FC<{ d: DiffDelta }> = ({ d }) => (
  <li className="border-t border-border-color px-4 py-3 first:border-t-0">
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
      <span
        className={`inline-block shrink-0 rounded-sm border px-1.5 py-px font-mono text-[11px] font-medium uppercase ${severityTone(
          d.severity
        )}`}
      >
        {d.severity}
      </span>
      <span className="font-mono text-xs text-text-muted">{d.kind}</span>
      <span className="font-mono text-sm text-text-main">{d.json_path}</span>
    </div>
    <p className="mt-1.5 text-sm text-text-secondary">{d.message}</p>
    {(d.expected || d.actual) && (
      <p className="mt-1 flex flex-wrap items-baseline gap-x-2 font-mono text-xs">
        <span className="text-text-muted">expected</span>
        <span className="text-text-main">{d.expected || '—'}</span>
        <span className="text-text-muted">actual</span>
        <span className="text-accent-breaking">{d.actual || '—'}</span>
      </p>
    )}
  </li>
);

const DeltaList: React.FC<{ diff: ContractDiff | null | undefined }> = ({ diff }) => {
  const deltas = diff?.deltas ?? [];
  if (!deltas.length) {
    return (
      <p className="px-4 py-3 text-sm text-text-muted">
        No deltas. The response matched the baseline exactly.
      </p>
    );
  }
  return (
    <ul>
      {deltas.map((d, i) => (
        <DeltaRow key={`${d.json_path}-${d.kind}-${i}`} d={d} />
      ))}
    </ul>
  );
};

/* A numbered stage of the run. Collapsed until it has something to show, so the
   page does not present three empty boxes and a spinner. */
const Stage: React.FC<{
  n: string;
  title: string;
  hint?: string;
  children?: React.ReactNode;
}> = ({ n, title, hint, children }) => (
  <section className="border-t border-border-color pt-6">
    <div className="flex items-baseline gap-3">
      <span className="font-mono text-xs text-text-muted">{n}</span>
      <h2 className="text-md font-semibold text-text-main">{title}</h2>
    </div>
    {hint && <p className="mt-2 pl-8 text-sm text-text-secondary">{hint}</p>}
    {children && <div className="mt-4 md:pl-8">{children}</div>}
  </section>
);

/** The result card: what was sent, what came back, and what Driftwood said. */
const ResultCard: React.FC<{ run: RunResult; waiting: boolean }> = ({ run, waiting }) => (
  <div className="overflow-hidden rounded-lg border border-border-color bg-bg-main">
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border-color bg-surface-3 px-4 py-2.5">
      <span className="font-mono text-xs text-text-main">GET {MOCK_PATH}</span>
      <span className="font-mono text-xs text-text-muted">{run.status}</span>
      {run.traffic ? (
        <span className={`font-mono text-xs font-medium ${statusTone(run.traffic.contract_status)}`}>
          {run.traffic.contract_status}
        </span>
      ) : (
        <span className="font-mono text-xs text-text-muted">
          {waiting ? 'waiting for the proxy to record it…' : 'not recorded'}
        </span>
      )}
      {run.traffic && (
        <span className="ml-auto font-mono text-xs text-text-muted">
          {run.traffic.duration_ms}ms
        </span>
      )}
    </div>

    {/* min-w-0 on both columns: a grid item's min-width is `auto`, so without
        it a long response body would set the track width and scroll the page
        sideways instead of scrolling inside its own box. */}
    <div className="grid md:grid-cols-2">
      <div className="min-w-0 border-b border-border-color md:border-b-0 md:border-r">
        <div className="px-4 pt-3 pb-1 font-mono text-[11px] uppercase tracking-wide text-text-muted">
          response body
        </div>
        <pre className="overflow-x-auto px-4 pb-3 font-mono text-xs leading-relaxed text-text-secondary">
          {prettyBody(run.body)}
        </pre>
      </div>
      <div className="min-w-0">
        <div className="px-4 pt-3 pb-1 font-mono text-[11px] uppercase tracking-wide text-text-muted">
          deltas against the baseline
        </div>
        <DeltaList diff={run.traffic?.diff} />
      </div>
    </div>
  </div>
);

export const TryItOut: React.FC = () => {
  const [phase, setPhase] = useState<Phase>('probing');
  const [mode, setMode] = useState<MockMode>('TYPE_BREAK');
  const [baseline, setBaseline] = useState<RunResult | null>(null);
  const [broken, setBroken] = useState<RunResult | null>(null);
  const [alert, setAlert] = useState<AlertEvent | null>(null);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  /* The traffic the server published for the request currently in flight.
     Held in a ref rather than state because it is a rendezvous, not something
     rendered: the run below writes a resolver here, the stream calls it, and
     neither should cause a re-render on the way past. */
  const waiter = useRef<((t: CapturedTraffic) => void) | null>(null);
  const modeRef = useRef<MockMode>(mode);
  modeRef.current = mode;

  useEffect(() => {
    let cancelled = false;
    probe().then((live) => {
      if (!cancelled) setPhase(live ? 'ready' : 'offline');
    });
    return () => {
      cancelled = true;
    };
  }, []);

  /* One stream for the life of the page, opened only once an instance has been
     confirmed. Two things depend on it: the traffic event that carries the diff
     for each request, and the alert event that shows what a BREAKING status
     triggers — which is the half of the product a diff table alone would not
     demonstrate. */
  useEffect(() => {
    if (phase !== 'ready') return;

    return openStream((msg: EventMessage) => {
      if (msg.type === 'traffic') {
        const t = msg.data as CapturedTraffic;
        if (t.path === MOCK_PATH && waiter.current) {
          const resolve = waiter.current;
          waiter.current = null;
          resolve(t);
        }
        return;
      }
      if (msg.type === 'alert') {
        setAlert(msg.data as AlertEvent);
      }
    });
  }, [phase]);

  /* Sends one request and waits for the proxy to publish what it made of it.
     The wait is bounded: if nothing arrives, the card says the request was not
     recorded rather than hanging or inventing a diff. */
  const runOnce = useCallback(async (m: MockMode): Promise<RunResult> => {
    await setMockMode(m);

    const recorded = new Promise<CapturedTraffic | null>((resolve) => {
      waiter.current = resolve;
      window.setTimeout(() => {
        if (waiter.current === resolve) {
          waiter.current = null;
          resolve(null);
        }
      }, 4000);
    });

    const res = await sendMockRequest();
    return { mode: m, status: res.status, body: res.body, traffic: await recorded };
  }, []);

  const runDemo = useCallback(async () => {
    setRunning(true);
    setError(null);
    setBaseline(null);
    setBroken(null);
    setAlert(null);

    try {
      // Stage 1 — a healthy response. This is what the baseline describes, and
      // doing it first is what makes stage 2 legible: the same endpoint, the
      // same call, one field different.
      setBaseline(await runOnce('NORMAL'));
      // Stage 2 — the chosen break.
      setBroken(await runOnce(modeRef.current));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRunning(false);
    }
  }, [runOnce]);

  if (phase === 'probing') {
    return (
      <Shell className="py-24">
        <p className="text-sm text-text-muted">Looking for a Driftwood instance…</p>
      </Shell>
    );
  }

  if (phase === 'offline') {
    return (
      <Shell className="py-20 md:py-28">
        <div className="max-w-2xl">
          <Badge tone="neutral">
            <span className="h-1.5 w-1.5 rounded-full bg-accent-warning" aria-hidden="true" />
            No instance on this origin
          </Badge>
          <h1 className="mt-6 text-3xl font-semibold tracking-tight text-text-main md:text-4xl">
            There is nothing to drive here
          </h1>
          <p className="mt-5 text-md text-text-secondary">
            This page drives a Driftwood instance over the control API, on the
            same origin it was served from — the control plane refuses
            cross-origin calls by design, so there is no other instance it could
            reach. It asked{' '}
            <span className="font-mono text-sm text-text-main">
              {CONTROL_PREFIX}/api/traffic
            </span>{' '}
            and got nothing that looked like a Driftwood reply.
          </p>
          <p className="mt-4 text-md text-text-secondary">
            Rather than show you a recording, it is saying so. Start an instance
            and reload:
          </p>
          <pre className="mt-6 overflow-x-auto rounded-md border border-border-color bg-bg-card px-4 py-3.5 font-mono text-xs leading-relaxed text-text-main">
{`git clone ${GITHUB_URL}.git
cd Driftwood
make build
./drift --port 8787`}
          </pre>
          <p className="mt-4 text-sm text-text-muted">
            Then serve this site behind the same origin, or open{' '}
            <span className="font-mono text-xs">http://127.0.0.1:8787/_driftwood/</span>{' '}
            for the dashboard itself, which has its own built-in simulator
            controls.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <LinkButton href="/" variant="secondary">
              Back to the overview
            </LinkButton>
            <LinkButton href={GITHUB_URL} variant="secondary" external>
              Read the source
            </LinkButton>
          </div>
        </div>
      </Shell>
    );
  }

  return (
    <Shell className="py-12 md:py-16">
      <div className="max-w-3xl">
        <Badge tone="info">
          <span className="h-1.5 w-1.5 rounded-full bg-accent-healthy" aria-hidden="true" />
          Live — connected to a Driftwood instance on this origin
        </Badge>
        <h1 className="mt-6 text-3xl font-semibold tracking-tight text-text-main md:text-4xl">
          Watch a contract break
        </h1>
        <p className="mt-5 text-md text-text-secondary">
          This drives the instance serving this page. The mock simulator is an
          endpoint Driftwood answers itself, so the traffic below is real
          traffic — proxied, recorded, diffed against a stored baseline, and
          published over the same event stream the dashboard reads.
        </p>
      </div>

      <div className="mt-12 space-y-6">
        <Stage
          n="01"
          title="Send a healthy response"
          hint="Driftwood's simulator is seeded with a baseline for this endpoint, so a normal call matches it."
        >
          {baseline && <ResultCard run={baseline} waiting={false} />}
        </Stage>

        <Stage
          n="02"
          title="Break it"
          hint="Same endpoint, same request. One field changes shape."
        >
          <div className="flex flex-wrap gap-2">
            {BREAK_MODES.map((m) => (
              <button
                key={m.mode}
                type="button"
                onClick={() => setMode(m.mode)}
                aria-pressed={mode === m.mode}
                className={`rounded-md border px-3 py-2 text-left transition-colors duration-150 ${
                  mode === m.mode
                    ? 'border-accent-primary bg-bg-card'
                    : 'border-border-color bg-bg-card hover:bg-bg-hover'
                }`}
              >
                <span className="block text-sm font-medium text-text-main">{m.label}</span>
                <span className="mt-0.5 block font-mono text-[11px] text-text-muted">
                  {m.blurb}
                </span>
              </button>
            ))}
          </div>

          {broken && (
            <div className="mt-4">
              <ResultCard run={broken} waiting={false} />
            </div>
          )}
        </Stage>

        <Stage
          n="03"
          title="What Driftwood did about it"
          hint="Only a delta that breaks a promise raises an alert. Everything is recorded; not everything is broadcast."
        >
          {alert ? (
            <div className="overflow-hidden rounded-lg border border-border-color bg-bg-card">
              <div className="border-b border-border-color bg-surface-3 px-4 py-2.5">
                <span className="font-mono text-xs text-text-muted">alert · event</span>
              </div>
              <div className="px-4 py-3">
                <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                  <span className="font-mono text-sm text-text-main">{alert.endpoint}</span>
                  <span
                    className={`font-mono text-xs font-medium ${statusTone(alert.contract_status)}`}
                  >
                    {alert.contract_status}
                  </span>
                  <span className="font-mono text-xs text-text-muted">
                    {alert.traffic_id}
                  </span>
                </div>
                <p className="mt-2 text-sm text-text-secondary">
                  Filed in the Alerts view and pushed to every open dashboard over
                  SSE. This is the event the page you are reading received.
                </p>
              </div>
            </div>
          ) : (
            <p className="text-sm text-text-muted">
              No alert raised for this run — which is the correct outcome when the
              change does not break anything. Add a field and watch this space
              stay empty.
            </p>
          )}
        </Stage>
      </div>

      {error && (
        <div
          role="alert"
          className="mt-8 rounded-md border border-accent-breaking bg-bg-card px-4 py-3 text-sm text-text-main"
        >
          <span className="font-medium">The run stopped:</span> {error}
        </div>
      )}

      <div className="mt-10 flex flex-wrap items-center gap-3 border-t border-border-color pt-8">
        <button
          type="button"
          onClick={runDemo}
          disabled={running}
          className="inline-flex items-center justify-center gap-2 rounded-md border border-transparent bg-accent-primary px-5 py-3 text-md font-medium whitespace-nowrap text-on-accent transition-colors duration-150 hover:brightness-110 disabled:opacity-60"
        >
          {running ? 'Running…' : baseline ? 'Run it again' : 'Run the sequence'}
        </button>
        <LinkButton href={`${CONTROL_PREFIX}/`} variant="secondary" size="lg">
          Open the full dashboard
        </LinkButton>
      </div>

      <p className="mt-5 max-w-2xl text-xs text-text-muted">
        The dashboard link shows this same traffic in the real UI — the table,
        the diff drawer, the alert list. Note that the simulator's mode is
        per-instance, so if someone else is using the same instance at the same
        moment the two of you are changing one setting between you.
      </p>
    </Shell>
  );
};
