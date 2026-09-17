import React, { useState } from 'react';
import { GITHUB_URL } from '../components/Nav';
import { Eyebrow, Section } from '../components/ui';

/* Questions a reader actually has after the three sections above, answered with
   the behaviour the code has.

   Two rules held here. Every answer is checkable in the repository, and the ones
   that are unflattering are still answered — "does it send my traffic
   anywhere", "what does it not do", "is it production-ready". A FAQ that only
   answers the easy questions is a feature list wearing a question mark. */

const FAQS: Array<{ q: string; a: React.ReactNode }> = [
  {
    q: 'Does Driftwood send my traffic anywhere?',
    a: (
      <>
        No. The proxy and dashboard make no outbound requests — there is no
        telemetry, no account, and no license check. The Go module has no
        dependencies at all, so there is not even a third-party client that
        could. The exception is the optional AI sidecar, which is a separate
        process you start yourself and which talks to a model API; if you do not
        start it, nothing leaves the machine.
      </>
    ),
  },
  {
    q: 'Is it safe to bind to my network?',
    a: (
      <>
        Not as it stands. The dashboard is an unauthenticated control plane —
        anyone who can reach it can rewrite the proxy target, delete baselines,
        and read captured traffic, which may include tokens and personal data
        that passed through. It binds to{' '}
        <span className="font-mono text-xs">127.0.0.1</span> by default and says so
        on startup. Remote access means a tunnel or a reverse proxy that
        authenticates in front of it.
      </>
    ),
  },
  {
    q: 'Can it replay a request or change a response?',
    a: (
      <>
        No, and that is deliberate. Driftwood is a sniffer: it reads what passes
        through and forwards it untouched. It is not a mock server, not a
        fault-injection tool, and not a load generator. The built-in simulator
        is the one thing that produces traffic rather than observing it, and it
        only answers on its own path.
      </>
    ),
  },
  {
    q: 'How does this compare to a schema test in CI?',
    a: (
      <>
        They catch different things. A contract test asserts what you remembered
        to write down, against a service you control, at the moment it runs.
        Driftwood watches what your API actually returns while you develop
        against it, and catches the change nobody wrote a test for — which is
        the change that breaks the frontend at 4pm on a Thursday. Running both
        is reasonable; they are not substitutes.
      </>
    ),
  },
  {
    q: 'What does it not do?',
    a: (
      <>
        It does not authenticate, does not encrypt, does not persist captured
        traffic beyond the in-memory window, and does not proxy WebSocket or
        streaming responses usefully — the diff model assumes a JSON body that
        has finished arriving. It stores up to 50 observations per endpoint, so
        a long-running instance keeps a recent window rather than a full
        history. It is a development tool that has been honest about being one.
      </>
    ),
  },
  {
    q: 'Is it production-ready?',
    a: (
      <>
        It is ready to run in front of a development environment, which is what
        it was built for. It has no authentication of its own, a deliberate
        absence rather than an oversight, and its storage is a JSON file in{' '}
        <span className="font-mono text-xs">~/.driftwood/</span>. Putting it in
        front of production traffic would mean accepting an unauthenticated
        control plane on that network, so: not without something in front of it.
      </>
    ),
  },
  {
    q: 'How do I get it?',
    a: (
      <>
        Clone the repository and run{' '}
        <span className="font-mono text-xs">make build</span>. It is not on the
        npm registry — the publish workflow runs on a GitHub Release and no
        release has been cut — so npm and npx are the intended interface rather
        than a working one. The{' '}
        <a href={GITHUB_URL} target="_blank" rel="noreferrer noopener">
          source
        </a>{' '}
        is the whole distribution for now.
      </>
    ),
  },
];

const Item: React.FC<{ q: string; a: React.ReactNode; open: boolean; onToggle: () => void }> = ({
  q,
  a,
  open,
  onToggle,
}) => (
  <div className="border-b border-border-color">
    <h3>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full items-start justify-between gap-6 py-5 text-left"
      >
        <span className="text-md font-medium text-text-main">{q}</span>
        {/* A drawn plus that becomes a minus by dropping its vertical bar. One
            element, two states, and it rotates rather than swapping glyphs, so
            the transition has something to interpolate. */}
        <span
          aria-hidden="true"
          className="relative mt-1.5 h-3.5 w-3.5 shrink-0 text-text-muted"
        >
          <span className="absolute top-1/2 left-0 h-px w-full -translate-y-1/2 bg-current" />
          <span
            className={`absolute top-0 left-1/2 h-full w-px -translate-x-1/2 bg-current transition-transform duration-200 ${
              open ? 'scale-y-0' : 'scale-y-100'
            }`}
          />
        </span>
      </button>
    </h3>
    {open && (
      <div className="max-w-2xl pr-10 pb-6 text-sm leading-relaxed text-text-secondary">{a}</div>
    )}
  </div>
);

export const Faq: React.FC = () => {
  const [open, setOpen] = useState<number | null>(0);

  return (
    <Section id="faq">
      <div className="grid gap-10 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:gap-14">
        <div>
          <Eyebrow>FAQ</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-text-main md:text-3xl">
            The questions worth asking
          </h2>
          <p className="mt-4 text-md text-text-secondary">
            Including the ones a product page usually avoids. If an answer here
            is wrong, it is a bug — the source is public and the behaviour is
            checkable.
          </p>
        </div>

        <div className="border-t border-border-color">
          {FAQS.map((f, i) => (
            <Item
              key={f.q}
              q={f.q}
              a={f.a}
              open={open === i}
              onToggle={() => setOpen(open === i ? null : i)}
            />
          ))}
        </div>
      </div>
    </Section>
  );
};
