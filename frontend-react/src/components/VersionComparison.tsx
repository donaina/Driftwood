import React, { useEffect, useState } from 'react';
import { Panel, PanelTitle } from './ui';
import type { HistoryItem } from './EndpointHistory';

/* The one component in this view that reads project-scoped state, which is why
   it is its own file: EndpointHistory is a presenter that takes its rows as
   props, and the guard in tests/shell_nav_test.go draws exactly that line — a
   component that names a project-scoped route and calls fetch() has to answer
   the shell's project-change announcement.

   What changed between two versions somebody selected.

   This panel used to say the feature "would require enhanced backend API" and
   show nothing at all. Neither half was true by the time anyone read it: every
   version the API sends already carries its schema, and diff.CompareSchemas was
   already the function the proxy compares a live response against. The missing
   piece was a route to ask one version about another, and that is
   /_driftwood/api/histories/diff. */

/* One measured difference, tagged the way pkg/types/types.go serialises
   DiffDelta.

   `severity` is the engine's and is never re-derived here. A view that worked
   severity out from the kind would be a second opinion about how bad something
   is, and the product's one job is telling a person when their contract moved —
   two answers to that is worse than a shrug. */
interface DiffDelta {
  json_path: string;
  kind: string;
  /* BREAKING, WARNING or INFO. */
  severity: string;
  message: string;
  /* Type names on either side of the change, or the engine's `<absent>`
     sentinel when the field exists on one side only. A STATUS_CODE_CHANGE
     carries "2xx" and the status that arrived instead. */
  expected: string;
  actual: string;
}

interface ContractDiff {
  has_breaking_changes: boolean;
  has_warnings: boolean;
  deltas: DiffDelta[];
}

/* The three severities pkg/types declares, in the order a reader wants them:
   what broke, what is at risk, what is merely new. */
const SEVERITY_ORDER = ['BREAKING', 'WARNING', 'INFO'];

const SEVERITY_NOUN: Record<string, string> = {
  BREAKING: 'breaking',
  WARNING: 'warning',
  INFO: 'informational',
};

/* Severity carries the claim, so severity carries the colour — except INFO,
   which is the one of the three that is not a problem and reads as a count
   rather than a verdict.

   §4 gives a delta a severity-tinted left bar and background, and the
   .delta-item classes below apply exactly that. The word is here for the channel
   colour cannot reach: greyscale, colourblindness, a screenshot pasted into a
   ticket. */
const SEVERITY_TONE: Record<string, string> = {
  BREAKING: 'text-accent-breaking',
  WARNING: 'text-accent-warning',
  INFO: 'text-text-muted',
};

/* A delta in the engine's own words: `message` is what happened ("removed",
   "type mismatch", or a whole sentence for a status change), and expected/actual
   name the two sides of it.

   `<absent>` is the engine's sentinel rather than a value, so it prints as the
   word — "string → absent" reads, "string → <absent>" reads like a rendering
   bug. Nothing here is inferred: a delta with neither side prints only its
   message. */
const describeDelta = (d: DiffDelta): string => {
  const side = (v: string) => (v === '<absent>' ? 'absent' : v);
  const move = !d.expected && !d.actual ? '' : `${side(d.expected)} → ${side(d.actual)}`;
  return [d.message, move].filter(Boolean).join(' · ');
};

type Comparison =
  | { status: 'loading' }
  | { status: 'failed'; detail: string }
  | { status: 'done'; diff: ContractDiff };

const VersionComparison: React.FC<{
  history: HistoryItem;
  selected: number[];
}> = ({ history, selected }) => {
  /* Oldest first, rather than in the order the two were clicked. The engine
     reads a diff as "what the baseline promised versus what arrived", so
     REMOVED_FIELD only means what a reader expects when the older version is the
     baseline. Clicking v3 and then v1 in a timeline must not turn three removed
     fields into three added ones. */
  const [from, to] = [...selected].sort((a, b) => a - b);
  const [comparison, setComparison] = useState<Comparison>({ status: 'loading' });

  /* The shell announces a project switch instead of remounting this view:
     mountView reuses the existing root and calls render on it, so React keeps
     the component instance and an effect with a fixed dependency list never runs
     a second time. Without this, a switch would leave the previous project's
     deltas on screen under the new project's name.

     It is not only about what is displayed. The selection is keyed by method and
     path with no project in it, so after a switch the same two version numbers
     mean a different project's versions — the panel has to go and ask again
     rather than answer from memory. */
  const [epoch, setEpoch] = useState(0);
  useEffect(() => {
    const onProjectChanged = () => setEpoch((n) => n + 1);
    window.addEventListener('driftwood:project-changed', onProjectChanged);
    return () => window.removeEventListener('driftwood:project-changed', onProjectChanged);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setComparison({ status: 'loading' });

    (async () => {
      try {
        const query = new URLSearchParams({
          method: history.method,
          path: history.path,
          from: String(from),
          to: String(to),
        });
        const res = await fetch(`/_driftwood/api/histories/diff?${query.toString()}`);
        if (!res.ok) {
          throw new Error(`${res.status} ${(await res.text()).trim()}`);
        }
        const diff: ContractDiff = await res.json();
        // A response can land after the selection moved on. Dropping it is the
        // difference between "the deltas for what you picked" and "the deltas
        // for what you picked a moment ago, under the same heading".
        if (!cancelled) setComparison({ status: 'done', diff });
      } catch (err) {
        if (!cancelled) setComparison({ status: 'failed', detail: String(err) });
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [history.method, history.path, from, to, epoch]);

  const renderBody = () => {
    if (comparison.status === 'failed') {
      /* Not the empty state, and it must never be able to look like one:
         "no structural difference" is a measurement, and a comparison that never
         happened has to say so in its own voice. */
      return (
        <p className="text-sm text-accent-breaking" role="alert">
          Could not compare v{from} and v{to}: {comparison.detail} — this is a
          comparison that did not happen, not one that found no differences.
        </p>
      );
    }

    if (comparison.status === 'loading') {
      return (
        <p className="text-sm text-text-muted">
          Comparing v{from} and v{to}…
        </p>
      );
    }

    const { deltas } = comparison.diff;
    if (deltas.length === 0) {
      return (
        <p className="delta-ok">
          ● No structural difference between v{from} and v{to}.
        </p>
      );
    }

    const counted = SEVERITY_ORDER.map((severity) => ({
      severity,
      count: deltas.filter((d) => d.severity === severity).length,
    })).filter((s) => s.count > 0);

    return (
      <>
        <p className="text-sm mb-3">
          <span className="text-text-main">
            {deltas.length} {deltas.length === 1 ? 'difference' : 'differences'}
          </span>
          <span className="text-text-muted"> — </span>
          {counted.map((s, i) => (
            <React.Fragment key={s.severity}>
              {i > 0 && <span className="text-text-muted"> · </span>}
              <span className={SEVERITY_TONE[s.severity]}>
                {s.count} {SEVERITY_NOUN[s.severity]}
              </span>
            </React.Fragment>
          ))}
        </p>
        {deltas.map((delta, index) => (
          <div
            key={`${delta.json_path}:${delta.kind}:${index}`}
            className={`delta-item ${delta.severity.toLowerCase()}`}
          >
            <div className="flex items-baseline justify-between gap-3">
              <span className="delta-path wrap-anywhere min-w-0">{delta.json_path}</span>
              <span
                className={`text-xs font-mono ${SEVERITY_TONE[delta.severity] ?? 'text-text-muted'}`}
              >
                {delta.severity}
              </span>
            </div>
            <div className="sp-top-xs">{describeDelta(delta)}</div>
          </div>
        ))}
      </>
    );
  };

  return (
    <Panel pad="sm">
      <PanelTitle className="mb-2">Version Comparison</PanelTitle>
      {/* Mono and muted, oldest on the left: the arrow is the direction the
          deltas below are read in, not an ornament. */}
      <div className="font-mono text-sm text-text-muted mb-3">
        v{from} → v{to}
      </div>
      {renderBody()}
    </Panel>
  );
};

export default VersionComparison;
