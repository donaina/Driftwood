import React from 'react';
import { GITHUB_URL } from '../components/Nav';
import { LinkButton, Section, SectionHead } from '../components/ui';

/* How to actually get it running.

   npm leads, and that is a change. This section used to open with a git clone
   and then explain that the npm route was the intended interface rather than a
   working one: `@donaina/driftwood` was unpublished, because the publish
   workflow runs on a GitHub Release and none had been cut. That stopped being
   true at 1.0.1. A page whose first command 404s is a worse first impression
   than an extra git clone, which is why it read the way it did — but the same
   reasoning now points the other way, and leaving it would send every reader
   through a build they do not need.

   The clone stays underneath rather than being dropped. It is the path for a
   platform with no prebuilt binary, and it is what someone who wants to read
   the source before running it will do anyway.

   The installer's own detail is stated instead of implied, because it is the
   part a reader cannot see from the command: the tarball carries no binary. */

const NPM = `npm install -g @donaina/driftwood
drift --port 8787 --target http://localhost:3000`;

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
      title="One command from a shell to a dashboard."
      lead="The dashboard is compiled into the binary, so a single file is the whole product — install it, copy it anywhere, run it. There is no service to register and nothing to configure behind it."
    />

    <div className="mt-12 grid gap-6 lg:grid-cols-2">
      {/* min-w-0 is load-bearing, not tidiness. A grid item's min-width is
          `auto`, which means it refuses to shrink below its min-content width —
          and min-content for a terminal line is the whole line, because it
          cannot wrap. Without this, the `<pre>` below pushes the grid track to
          its own width and the page scrolls sideways at 375 while the code
          block's own overflow-x-auto never gets a chance to do its job. */}
      <div className="flex min-w-0 flex-col gap-4">
        <Code label="install">{NPM}</Code>
        <p className="text-sm text-text-secondary">
          The target does not have to exist. Driftwood serves a mock endpoint of
          its own and intercepts it before any dial, so this runs with nothing
          behind it — which is what makes a first contract, and a first breaking
          change, something you can watch happen without an API to point at.
        </p>
      </div>

      <div className="flex min-w-0 flex-col gap-4">
        <Code label="or from a clone">{CLONE}</Code>
        <p className="text-sm text-text-secondary">
          <span className="font-mono text-xs text-text-muted">make build</span> installs
          the dashboard’s dependencies, builds it, and then builds the binary — in
          that order, because the binary embeds the dashboard at compile time.
        </p>
      </div>
    </div>

    {/* The install detail is addressed rather than omitted: it is the one part
        of the command above a reader cannot verify by reading it, and it is the
        part that decides whether the install works on their machine. */}
    <div className="mt-6 rounded-lg border border-border-color bg-surface-3 p-6">
      <h3 className="text-sm font-semibold text-text-main">What the install actually does</h3>
      <div className="mt-3 grid gap-4 md:grid-cols-2">
        <p className="text-sm text-text-secondary">
          The tarball carries no binary. Its{' '}
          <span className="font-mono text-xs text-text-main">postinstall</span> downloads the
          build for your platform from the GitHub Release matching the package’s own
          version, checks its SHA256 against that release’s{' '}
          <span className="font-mono text-xs text-text-main">SHA256SUMS.txt</span>, and only
          then marks it executable. The checksum is the point: a download that was
          truncated or substituted fails the install instead of reaching your shell.
        </p>
        <p className="text-sm text-text-secondary">
          If the download fails it falls back to a local{' '}
          <span className="font-mono text-xs">go build</span>, which needs Go 1.25 or
          newer — so on a machine with neither, the install reports the step that
          failed rather than exiting successfully having installed nothing. Prebuilt
          binaries cover macOS, Linux and Windows on{' '}
          <span className="font-mono text-xs">amd64</span> and{' '}
          <span className="font-mono text-xs">arm64</span>; anything else takes the
          clone path on the left.
        </p>
      </div>
      <p className="mt-4 text-sm text-text-secondary">
        The by-hand equivalent of{' '}
        <span className="font-mono text-xs text-text-muted">make build</span>, if you
        would rather run the steps yourself:
      </p>
      <div className="mt-3">
        <Code label="the same thing, by hand">{BY_HAND}</Code>
      </div>
      <p className="mt-3 text-sm text-text-secondary">
        A committed placeholder keeps the embed compiling on a fresh clone, so this
        is the step that is easy to skip — the build still succeeds, and the
        dashboard renders unstyled and inert. If that happens, the binary says so at
        startup rather than leaving you to guess.
      </p>
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
