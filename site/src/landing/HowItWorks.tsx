import React from 'react';
import { Section, SectionHead } from '../components/ui';

/* The mechanism, in three steps, stated as what the software does rather than
   what it would be nice to say it does.
 *
 * The order is the product's actual order: the baseline has to exist before a
 * comparison can be made, and the comparison has to exist before severity means
 * anything. Every noun here is a real one — the reverse proxy, the inferred
 * schema, the stored baseline, the delta — because a reader who goes on to run
 * the thing should not find different words waiting for them.
 *
 * LAID OUT AS A RAIL, NOT AS THREE CARDS. Three equal cards side by side is the
 * shape every feature section on the internet has, and it is banned here by
 * §7 — but the better reason is that it would be wrong about the content. These
 * three are a sequence with a direction, and equal-weight boxes in a row say
 * the opposite: that they are three independent things you might pick from.
 * The rail says what is true, that step 2 cannot happen without step 1.
 *
 * It is also the same visual language as the try-it-out page's numbered stages,
 * which is deliberate: the reader meets this section, then goes there and does
 * the thing it described, and the numbering should look like the same idea. */

type Step = {
  n: string;
  title: string;
  body: string;
  detail: string;
};

const STEPS: Step[] = [
  {
    n: '01',
    title: 'Point it at your API',
    body: 'Driftwood is a reverse proxy. You start it on a port, name the API behind it, and send your app at Driftwood instead. Nothing about your request or the response is rewritten on the way through.',
    detail: './drift --port 8787 --target http://localhost:3000',
  },
  {
    n: '02',
    title: 'It remembers the shape',
    body: 'Every JSON response is read for its structure — the types, the nesting, which keys are always present — and the result is stored as that endpoint’s baseline. The response itself is never the baseline; only the shape of it is.',
    detail: '~/.driftwood/baselines.json',
  },
  {
    n: '03',
    title: 'It compares everything after',
    body: 'Each later response is diffed against its baseline and every difference is recorded as a delta with a severity. Only a delta that breaks a promise raises an alert, which is the difference between a tool you leave running and one you mute.',
    detail: 'BREAKING · WARNING · INFO',
  },
];

export const HowItWorks: React.FC = () => (
  <Section id="how-it-works" className="bg-bg-card">
    <SectionHead
      eyebrow="How it works"
      title="A proxy, a baseline, and a comparison."
      lead="No agent to install in your services, no schema to write by hand, and nothing to change in the API itself. Driftwood learns the contract from traffic it has already seen."
    />

    <ol className="mt-12 max-w-3xl">
      {STEPS.map((s, i) => {
        const last = i === STEPS.length - 1;
        return (
          <li key={s.n} className="grid grid-cols-[2rem_minmax(0,1fr)] gap-x-5 md:gap-x-7">
            {/* The rail. The connector is a flex-1 rule under the marker, so it
                takes whatever height the row's text gives it and stops at the
                last step rather than trailing off the end of the list. */}
            <div className="flex flex-col items-center">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-border-strong bg-bg-card font-mono text-xs font-medium text-text-secondary">
                {s.n}
              </span>
              {!last && <span className="mt-2 w-px flex-1 bg-border-color" aria-hidden="true" />}
            </div>

            <div className={last ? 'min-w-0' : 'min-w-0 pb-10'}>
              <h3 className="pt-1 text-md font-semibold text-text-main">{s.title}</h3>
              <p className="mt-2 text-sm text-text-secondary">{s.body}</p>
              <p className="mt-4 inline-block max-w-full truncate rounded-sm border border-border-color bg-surface-3 px-2.5 py-1 font-mono text-xs text-text-muted">
                {s.detail}
              </p>
            </div>
          </li>
        );
      })}
    </ol>

    {/* The limit of the inference, said here rather than in the FAQ, because it
        is the thing a reader will otherwise discover by being surprised. */}
    <div className="mt-6 rounded-lg border border-border-color bg-bg-main p-6 md:ml-[3.25rem]">
      <h3 className="text-sm font-semibold text-text-main">
        What a baseline inferred from traffic can and cannot promise
      </h3>
      <p className="mt-3 max-w-3xl text-sm text-text-secondary">
        One response cannot tell you whether a field is one the API always sends
        or one it happened to send that day, so a baseline learned from traffic
        treats every key it saw as required — and a key that disappears is then
        reported as breaking. A contract imported from an OpenAPI document knows
        better, because the document says which properties are required, and a
        property outside that list is reported as a warning instead. Both are
        honest about what they know; they just know different amounts.
      </p>
    </div>
  </Section>
);
