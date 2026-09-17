import React from 'react';
import { Shell } from '../components/ui';

/* A strip of facts, in the slot a logo wall or a rating row would occupy.

   It is not a logo wall because there are no customers to name, and inventing
   them is the one thing a page like this must not do. Every number below is a
   property of the code, and each is checkable in the repository:

   - go.mod has no require block at all.
   - The dashboard is //go:embed-ed into the binary, so it is one file.
   - maxObservations is 50 in internal/storage/storage.go.
   - The proxy and dashboard make no outbound requests. The optional AI sidecar
     is the only component that talks to a network, and it is off unless
     started. "Nothing leaves your machine" would be false without that
     qualifier, which is why the qualifier is there. */

const FACTS = [
  {
    value: '0',
    label: 'dependencies',
    detail: 'The Go module requires nothing. No framework, no driver, no ORM.',
  },
  {
    value: '1',
    label: 'binary to run',
    detail: 'The proxy and its dashboard compile into a single file you can copy anywhere.',
  },
  {
    value: '50',
    label: 'requests per endpoint',
    detail: 'The window each stability trend is computed over. A ring buffer, not a growing log.',
  },
  {
    value: '0',
    label: 'outbound calls',
    detail: 'The proxy and dashboard never dial out. The optional AI sidecar is the only piece that does, and it is off unless you start it.',
  },
];

export const Facts: React.FC = () => (
  <div className="rule py-12 md:py-14">
    <Shell>
      <dl className="grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
        {FACTS.map((f) => (
          <div key={f.label}>
            <dt className="sr-only">{f.label}</dt>
            <dd>
              <div className="flex items-baseline gap-2">
                <span className="font-mono text-2xl font-semibold text-text-main">{f.value}</span>
                <span className="text-sm text-text-secondary">{f.label}</span>
              </div>
              <p className="mt-2 text-xs text-text-muted">{f.detail}</p>
            </dd>
          </div>
        ))}
      </dl>
    </Shell>
  </div>
);
