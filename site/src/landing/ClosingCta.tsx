import React from 'react';
import { GITHUB_URL } from '../components/Nav';
import { LinkButton, Shell } from '../components/ui';

/* The last thing on the page, and the only place it repeats an ask.

   It repeats the two routes out rather than inventing a third: watch a real
   breaking change with nothing installed, or clone it and point it at your own
   API. A closing section that adds a newsletter box and a "book a demo" would
   be offering things this project does not have. */
export const ClosingCta: React.FC = () => (
  <div className="rule bg-surface-3 py-16 md:py-24">
    <Shell>
      <div className="mx-auto max-w-2xl text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-text-main md:text-3xl">
          See it catch one before you install anything
        </h2>
        <p className="mt-4 text-md text-text-secondary">
          The try-it-out page drives a real breaking change through a real
          Driftwood instance and shows you the diff it computes. Then clone it
          and point it at the API you are actually worried about.
        </p>
        <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <LinkButton href="/try" variant="primary" size="lg">
            Try it in your browser
          </LinkButton>
          <LinkButton href={GITHUB_URL} variant="secondary" size="lg" external>
            Clone the source
          </LinkButton>
        </div>
      </div>
    </Shell>
  </div>
);
