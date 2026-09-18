import React from 'react';
import { GITHUB_URL } from '../components/Nav';
import { Badge, LinkButton, Shell } from '../components/ui';

/* The dashboard, in a frame.

   Both themes ship, so the picture has to be able to be either. The two images
   are stacked and swapped by CSS rather than by React state: the theme is
   already decided before first paint by the inline script in <head>, and
   reading it into state here would mean rendering the wrong one first — a flash
   of the light screenshot on a dark page, which is exactly the failure the
   pre-paint script exists to prevent.

   The .theme-light-only / .theme-dark-only pair in styles.css does that, in
   CSS, with no state and no flash. Tailwind's `dark:` variant cannot: it follows
   the OS and would ignore an explicit choice. */
const DashboardFrame: React.FC = () => (
  <figure className="mt-14 md:mt-16">
    <div className="overflow-hidden rounded-lg border border-border-color bg-bg-card shadow-lg">
      {/* A drawn window bar rather than a screenshot of a browser: it crops the
          image without implying the product is a website. */}
      <div className="flex items-center gap-2 border-b border-border-color bg-surface-3 px-4 py-2.5">
        <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
        <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
        <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
        <span className="ml-2 font-mono text-xs text-text-muted">
          localhost:8787/_driftwood/
        </span>
      </div>
      <div className="relative aspect-[1440/900] w-full bg-bg-main">
        <img
          src="/dashboard-light.png"
          alt="The Driftwood dashboard: request rate, error rate and contract health across the top, and a live traffic table below showing requests marked MATCH and BREAKING."
          className="theme-light-only absolute inset-0 h-full w-full object-cover object-top"
          width={1440}
          height={900}
        />
        <img
          src="/dashboard-dark.png"
          alt="The Driftwood dashboard in its dark theme, showing the same live traffic table with requests marked MATCH and BREAKING."
          className="theme-dark-only absolute inset-0 h-full w-full object-cover object-top"
          width={1440}
          height={900}
        />
      </div>
    </div>
    <figcaption className="mt-3 text-center text-xs text-text-muted">
      The shipped dashboard, captured from a running binary. The breaking changes
      in that table were produced by its own simulator.
    </figcaption>
  </figure>
);

export const Hero: React.FC = () => (
  <div id="top" className="pt-16 pb-4 md:pt-24">
    <Shell>
      <div className="mx-auto max-w-3xl text-center">
        <Badge tone="info">
          <span className="inline-block h-1.5 w-1.5 rounded-full bg-accent-healthy" aria-hidden="true" />
          Open source · runs on your machine
        </Badge>

        <h1 className="mt-6 text-display font-semibold leading-[1.08] tracking-tight text-text-main md:text-display-lg">
          Catch the API change that
          <br className="hidden sm:block" /> breaks your frontend
        </h1>

        <p className="mx-auto mt-6 max-w-2xl text-md text-text-secondary md:text-lg">
          Driftwood sits between your app and its API, remembers the shape of
          every response it sees, and tells you the moment one of them stops
          matching — with the exact field that changed and what it changed from.
        </p>

        <div className="mt-9 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <LinkButton href="/try" variant="primary" size="lg">
            Try it in your browser
          </LinkButton>
          <LinkButton href={GITHUB_URL} variant="secondary" size="lg" external>
            Read the source
          </LinkButton>
        </div>

        <p className="mt-4 text-xs text-text-muted">
          The try-it-out page drives a real breaking change through a real
          Driftwood instance. Nothing to install, and nothing is sent anywhere.
        </p>
      </div>

      <DashboardFrame />
    </Shell>
  </div>
);
