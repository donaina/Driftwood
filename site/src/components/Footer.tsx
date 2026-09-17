import React from 'react';
import { GITHUB_URL } from './Nav';
import { Mark, Shell } from './ui';

/* The footer says what the software is and where it comes from, and nothing
   else. No social links to accounts that do not exist, no "trusted by" strip,
   no newsletter box wired to nothing. */
export const Footer: React.FC = () => (
  <footer className="rule py-12">
    <Shell>
      <div className="flex flex-col gap-8 md:flex-row md:items-start md:justify-between">
        <div className="max-w-sm">
          <div className="flex items-center gap-2.5">
            <Mark size={22} />
            <span className="font-semibold tracking-tight text-text-main">Driftwood</span>
          </div>
          <p className="mt-3 text-sm text-text-muted">
            A local proxy and dashboard that catches API contract drift before it
            reaches your users. Runs on your machine, on your traffic.
          </p>
        </div>

        <nav className="flex gap-12" aria-label="Footer">
          <div>
            <div className="eyebrow">Project</div>
            <ul className="mt-3 space-y-2 text-sm">
              <li>
                <a href={GITHUB_URL} target="_blank" rel="noreferrer noopener" className="text-text-secondary no-underline hover:text-text-main">
                  Source on GitHub
                </a>
              </li>
              <li>
                <a href={`${GITHUB_URL}/blob/main/README.md`} target="_blank" rel="noreferrer noopener" className="text-text-secondary no-underline hover:text-text-main">
                  README
                </a>
              </li>
              <li>
                <a href={`${GITHUB_URL}/blob/main/DESIGN.md`} target="_blank" rel="noreferrer noopener" className="text-text-secondary no-underline hover:text-text-main">
                  Design notes
                </a>
              </li>
            </ul>
          </div>
          <div>
            <div className="eyebrow">On this page</div>
            <ul className="mt-3 space-y-2 text-sm">
              <li>
                <a href="#what-it-catches" className="text-text-secondary no-underline hover:text-text-main">
                  What it catches
                </a>
              </li>
              <li>
                <a href="#how-it-works" className="text-text-secondary no-underline hover:text-text-main">
                  How it works
                </a>
              </li>
              <li>
                <a href="/try.html" className="text-text-secondary no-underline hover:text-text-main">
                  Try it out
                </a>
              </li>
            </ul>
          </div>
        </nav>
      </div>

      <div className="rule mt-10 pt-6 text-xs text-text-muted">
        Driftwood is open source. The dashboard in every screenshot on this page is
        the shipped one, captured from a running binary.
      </div>
    </Shell>
  </footer>
);
