import React from 'react';
import { Section, SectionHead } from '../components/ui';

/* What starting it actually prints.
 *
 * Copied from a real run, not written for the page. That is worth insisting on
 * because a marketing page's terminal output is usually an aspiration — the
 * flags get renamed, the banner gets prettier, and the transcript quietly
 * becomes fiction. This one is checked against the binary.
 *
 * The four `[Driftwood]` lines are verbatim; the ASCII banner above them is
 * omitted, and the caption says so. It is omitted because it is the one part of
 * the output that cannot survive being read on a page: it is a row of `=`
 * characters wide enough to wrap, and a wrapped banner reads as a rendering bug
 * rather than as output.
 *
 * The lines WRAP rather than scroll. An earlier version put them in an
 * overflow-x-auto box, which silently cut every long line at the container edge
 * — including the port number in the first line, which is the one value on it
 * that matters. Terminal output scrolls horizontally; prose on a page does not,
 * and the reader cannot tell that a scroll container is hiding the rest of a
 * line unless they happen to drag it.
 *
 * The loopback warning is kept in deliberately. It is the least flattering line
 * here — it is the program telling you its own dashboard has no authentication
 * — and it is exactly the kind of thing a page would normally cut. Cutting it
 * would leave a reader to discover it from the source after they had already
 * bound the thing to 0.0.0.0. */

const LINES: Array<{ time: string; text: string; note?: string }> = [
  {
    time: '22:58:55',
    text: '[Driftwood] Web Dashboard & Proxy running on http://127.0.0.1:18910',
    note: 'One port serves both the proxy and the dashboard. Send your app here.',
  },
  {
    time: '22:58:55',
    text: '[Driftwood] Bound to loopback only. Pass --host 0.0.0.0 to serve the network; the control plane has no authentication of its own.',
    note: 'The dashboard is an unauthenticated control plane, so it stays on loopback until you decide otherwise.',
  },
  {
    time: '22:58:55',
    text: '[Driftwood] Intercepting & forwarding traffic to http://localhost:3000',
    note: 'Where traffic is forwarded. Nothing on the way through is rewritten.',
  },
  {
    time: '22:58:55',
    text: '[Driftwood] Built-in Mock Simulator: http://localhost:18910/_driftwood/mock/users',
    note: 'A simulator it serves itself, so there is something to compare before you have pointed it at anything real.',
  },
];

export const Transcript: React.FC = () => (
  <Section>
    <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] lg:items-start lg:gap-14">
      <SectionHead
        eyebrow="What it says"
        title="It tells you what it is doing, and what it will not."
        lead="Four lines of substance on startup — and the one about the control plane having no authentication is the one that matters. Read that before you bind it to anything but loopback."
      />

      <figure className="min-w-0 overflow-hidden rounded-lg border border-border-color bg-bg-card">
        <div className="flex items-center gap-2 border-b border-border-color bg-surface-3 px-4 py-2.5">
          <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
          <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
          <span className="h-2.5 w-2.5 rounded-full bg-border-strong" aria-hidden="true" />
          <span className="ml-2 truncate font-mono text-xs text-text-muted">
            ./drift --port 18910 --target http://localhost:3000
          </span>
        </div>

        <div className="px-4 py-4">
          <ol>
            {LINES.map((l, i) => (
              <li key={i} className={i > 0 ? 'mt-4' : ''}>
                <div className="flex flex-col gap-x-3 gap-y-0.5 sm:flex-row">
                  <span className="shrink-0 font-mono text-xs leading-relaxed text-text-muted">
                    {l.time}
                  </span>
                  {/* break-words so an unbroken URL can still wrap rather than
                      push the box wider than its column. */}
                  <span className="font-mono text-xs leading-relaxed break-words text-text-main">
                    {l.text}
                  </span>
                </div>
                {l.note && (
                  <p className="mt-1 text-xs text-text-muted sm:pl-[4.5rem]">{l.note}</p>
                )}
              </li>
            ))}
          </ol>
        </div>

        <figcaption className="border-t border-border-color px-4 py-2.5 text-xs text-text-muted">
          Captured from a running binary; only the port was changed. The ASCII
          banner the program prints above these lines is left out — it is a row of
          <span className="font-mono"> = </span> signs wide enough to wrap, and a
          wrapped banner reads as a broken page rather than as output.
        </figcaption>
      </figure>
    </div>
  </Section>
);
