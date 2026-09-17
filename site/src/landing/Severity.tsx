import React from 'react';
import { Section, SectionHead } from '../components/ui';

/* What Driftwood catches, and — the harder half — what it refuses to call a
   break.

   This is the section the whole page turns on, because a diff is easy and a
   useful diff is not. Every entry below is the shipped behaviour: the severity
   classes are types.DiffSeverity, and the examples are the deltas the built-in
   simulator actually produces, captured from a running binary.

   The volatile-field note is here rather than in the FAQ because it is the
   difference between a tool you leave running and one you turn off after a day:
   a request ID changes on every call by design, and a checker that reports it
   is a checker that reports nothing but noise. */

type Row = {
  severity: 'breaking' | 'warning' | 'info';
  tone: string;
  title: string;
  examples: Array<{ from: string; to: string; note: string }>;
  outcome: string;
};

const ROWS: Row[] = [
  {
    severity: 'breaking',
    tone: 'text-accent-breaking border-accent-breaking',
    title: 'Breaks a promise the contract made',
    examples: [
      { from: '99812', to: '"99812"', note: 'integer → string' },
      { from: '$.email present', to: '$.email removed', note: 'required field gone' },
      { from: '"alex@company.com"', to: 'null', note: 'non-nullable returned null' },
      { from: '2024-01-31', to: '3f2a9c1e-…', note: 'known format changed' },
    ],
    outcome: 'Raises an alert and marks the request BREAKING.',
  },
  {
    severity: 'warning',
    tone: 'text-accent-warning border-accent-warning',
    title: 'Weakens the contract without breaking it',
    examples: [
      { from: '$.nickname present', to: '$.nickname absent', note: 'field the baseline never required' },
      { from: 'integer', to: 'number', note: 'widened, still numeric' },
      { from: '2024-01-31', to: '31 January 2024', note: 'stopped matching any known format' },
    ],
    outcome: 'Files the request in the Alerts view as WARNING. Nothing is broadcast.',
  },
  {
    severity: 'info',
    tone: 'text-accent-info border-accent-info',
    title: 'Keeps every promise and adds to them',
    examples: [
      { from: 'no $.new_feature', to: '$.new_feature: true', note: 'new property' },
      { from: 'null', to: '"active"', note: 'started returning a value' },
    ],
    outcome: 'Counted as healthy — the request still reads MATCH.',
  },
];

export const Severity: React.FC = () => (
  <Section id="what-it-catches">
    <SectionHead
      eyebrow="What it catches"
      title="A diff is easy. Knowing which changes matter is the product."
      lead="Every difference between a response and its baseline becomes a delta with a severity. Severity is what decides whether anything happens next — because a checker that treats every change as an emergency is a checker you turn off."
    />

    <div className="mt-12 space-y-4">
      {ROWS.map((row) => (
        <div
          key={row.severity}
          className="rounded-lg border border-border-color bg-bg-card p-6 md:p-7"
        >
          <div className="flex flex-col gap-5 md:flex-row md:items-start md:gap-8">
            <div className="md:w-64 md:shrink-0">
              <span
                className={`inline-block rounded-sm border px-2 py-0.5 font-mono text-xs font-medium uppercase tracking-wide ${row.tone}`}
              >
                {row.severity}
              </span>
              <h3 className="mt-3 text-md font-semibold text-text-main">{row.title}</h3>
            </div>

            <div className="min-w-0 flex-1">
              <ul className="space-y-2">
                {row.examples.map((ex) => (
                  <li key={ex.note} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 font-mono text-sm">
                    <span className="text-text-muted line-through decoration-text-muted/50">{ex.from}</span>
                    <span aria-hidden="true" className="text-text-muted">→</span>
                    <span className="text-text-main">{ex.to}</span>
                    <span className="font-sans text-xs text-text-muted">{ex.note}</span>
                  </li>
                ))}
              </ul>
              <p className="mt-4 text-sm text-text-secondary">{row.outcome}</p>
            </div>
          </div>
        </div>
      ))}
    </div>

    {/* The exemption, given its own weight because it is the feature that makes
        the other three usable. */}
    <div className="mt-4 rounded-lg border border-border-color bg-surface-3 p-6 md:p-7">
      <div className="flex flex-col gap-5 md:flex-row md:items-start md:gap-8">
        <div className="md:w-64 md:shrink-0">
          <span className="inline-block rounded-sm border border-border-strong px-2 py-0.5 font-mono text-xs font-medium uppercase tracking-wide text-text-secondary">
            set aside
          </span>
          <h3 className="mt-3 text-md font-semibold text-text-main">
            Values that change on every call
          </h3>
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm text-text-secondary">
            A field whose name marks it as noise — a request ID, a trace ID, a
            timestamp — is skipped before the comparison happens. Its value is
            different on every call by design, so a difference there is not
            drift, and a field that is nothing but noise must not be allowed to
            report a broken contract.
          </p>
          <p className="mt-4 font-mono text-sm text-text-muted">
            request_id · req_id · trace_id · span_id · correlation_id · nonce · etag
          </p>
          <p className="mt-1 font-mono text-sm text-text-muted">
            timestamp · time · created_at · updated_at · server_time
          </p>
          <p className="mt-3 text-xs text-text-muted">
            Matched case-insensitively, at any depth. An id the pattern does not
            cover is still just a field — name it something the list recognises,
            or accept that it will be compared.
          </p>
        </div>
      </div>
    </div>
  </Section>
);
