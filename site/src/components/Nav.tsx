import React, { useEffect, useState } from 'react';
import { LinkButton, Mark, Shell, ThemeToggle } from './ui';

export const GITHUB_URL = 'https://github.com/donaina/Driftwood';

const LINKS = [
  { href: '#what-it-catches', label: 'What it catches' },
  { href: '#how-it-works', label: 'How it works' },
  { href: '#run-it', label: 'Run it' },
  { href: '#faq', label: 'FAQ' },
];

/* The site's header. It is sticky because the page is long and the two things
   worth reaching from anywhere on it are "try it" and the source.

   The dark border only appears once the page has scrolled: at the top the
   header sits on the same ground as the hero and a rule under it would draw a
   line across nothing. */
export const Nav: React.FC = () => {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  return (
    <header
      className={`sticky top-0 z-40 transition-colors duration-200 ${
        scrolled ? 'border-b border-border-color bg-bg-main/90 backdrop-blur' : 'border-b border-transparent'
      }`}
    >
      <Shell className="flex h-16 items-center justify-between gap-6">
        <a href="#top" className="flex items-center gap-2.5 no-underline">
          <Mark />
          <span className="text-md font-semibold tracking-tight text-text-main">Driftwood</span>
        </a>

        {/* The section links are decorative repetition on a narrow screen: the
            page is short enough to scroll and a collapsed menu would hide more
            than it saves. The two that matter — try it, and the source — stay. */}
        <nav className="hidden items-center gap-7 md:flex" aria-label="Sections">
          {LINKS.map((l) => (
            <a
              key={l.href}
              href={l.href}
              className="text-sm text-text-secondary no-underline transition-colors duration-150 hover:text-text-main"
            >
              {l.label}
            </a>
          ))}
        </nav>

        <div className="flex items-center gap-2">
          <ThemeToggle />
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer noopener"
            className="hidden text-sm text-text-secondary no-underline transition-colors duration-150 hover:text-text-main sm:inline"
          >
            Source
          </a>
          <LinkButton href="/try" variant="primary">
            Try it out
          </LinkButton>
        </div>
      </Shell>
    </header>
  );
};
