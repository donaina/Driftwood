import React from 'react';
import { GITHUB_URL } from '../components/Nav';
import { LinkButton, Section, SectionHead } from '../components/ui';

/* How to actually get it running.

   The npm route is described and then immediately ruled out, because that is
   the truth: the publish workflow runs on a GitHub Release, no release has been
   cut, and `@donaina/driftwood` is not on the registry. A page that offered
   `npx @donaina/driftwood` as the first command would be sending every reader
   into a 404 on their first interaction with the product — which is a worse
   first impression than an extra git clone. */

const CLONE = `git clone https://github.com/donaina/Driftwood.git
cd Driftwood
make build
./drift --port 8787 --target http://localhost:3000`;

const BY_HAND = `npm --prefix frontend-react ci
npm --prefix frontend-react run build
go build -o drift ./cmd/drift`;

const Code: React.FC<{ children: string; label: string }> = ({ children, label }) => (
  <div className="overflow-hidden rounded-md border border-border-color bg-bg-main">
    <div className="flex items-center justify-between border-b border-border-color px-4 py-2">
      <span className="font-mono text-xs text-text-muted">{label}</span>
      {/* No copy button: it would be a lie on an insecure origin, where the
          clipboard API is unavailable, and a flash of "Copied" that did not
          copy is exactly the kind of small dishonesty this page is trying not
          to commit. Selecting the text works everywhere. */}
    </div>
    <pre className="overflow-x-auto px-4 py-3.5 font-mono text-xs leading-relaxed text-text-main">
      {children}
    </pre>
  </div>
);

export const RunIt: React.FC = () => (
  <Section id="run-it">
    <SectionHead
      eyebrow="Run it"
      title="One command from a clone to a dashboard."
      lead="There is no installer and no service to register. The dashboard is compiled into the binary, so a single file is the whole product — copy it anywhere and run it."
    />

    <div className="mt-12 grid gap-6 lg:grid-cols-2">
      {/* min-w-0 is load-bearing, not tidiness. A grid item's min-width is
          `auto`, which means it refuses to shrink below its min-content width —
          and min-content for a terminal line is the whole line, because it
          cannot wrap. Without this, the `<pre>` below pushes the grid track to
          its own width and the page scrolls sideways at 375 while the code
          block's own overflow-x-auto never gets a chance to do its job. */}
      <div className="flex min-w-0 flex-col gap-4">
        <Code label="shell">{CLONE}</Code>
        <p className="text-sm text-text-secondary">
          <span className="font-mono text-xs text-text-muted">make build</span> installs
          the dashboard’s dependencies, builds it, and then builds the binary — in
          that order, because the binary embeds the dashboard at compile time.
        </p>
      </div>

      <div className="flex min-w-0 flex-col gap-4">
        <Code label="the same thing, by hand">{BY_HAND}</Code>
        <p className="text-sm text-text-secondary">
          A committed placeholder keeps the embed compiling on a fresh clone, so
          this is the step that is easy to skip — the build still succeeds, and
          the dashboard renders unstyled and inert. If that happens, the binary
          says so at startup rather than leaving you to guess.
        </p>
      </div>
    </div>

    {/* npm is the interface people will look for first, so it is addressed
        rather than omitted. */}
    <div className="mt-6 rounded-lg border border-border-color bg-surface-3 p-6">
      <h3 className="text-sm font-semibold text-text-main">
        Not on the npm registry yet
      </h3>
      <div className="mt-3 grid gap-4 md:grid-cols-2">
        <p className="text-sm text-text-secondary">
          <span className="font-mono text-xs text-text-main">@donaina/driftwood</span> has
          never been published. The publish workflow runs when a GitHub Release is
          created, and Actions on this repository currently fails on a billing
          lock, so no release has been cut — which means the familiar{' '}
          <span className="font-mono text-xs">npx</span> and{' '}
          <span className="font-mono text-xs">npm install -g</span> commands are the
          intended interface rather than a working one. Clone and build above in
          the meantime.
        </p>
        <p className="text-sm text-text-secondary">
          A Node module and an Express integration are written and in the
          repository; they are waiting on the same release. Everything else on
          this page works today, from a clone.
        </p>
      </div>
    </div>

    <div className="mt-8 flex flex-wrap items-center gap-3">
      <LinkButton href={GITHUB_URL} variant="primary" size="lg" external>
        Read the source
      </LinkButton>
      <LinkButton href="/try" variant="secondary" size="lg">
        Or try it without installing
      </LinkButton>
    </div>
  </Section>
);
