import React, { useCallback, useEffect, useState } from 'react';
import { applyTheme, currentTheme, type Theme } from '../lib/theme';

/* The site's primitives.

   Deliberately few. A marketing page does not need a component library, and
   every primitive added here is one more thing that can drift from the
   dashboard's own look. These read entirely from the tokens, so they follow it
   by construction rather than by being kept in sync. */

export const Shell: React.FC<{ children: React.ReactNode; className?: string }> = ({
  children,
  className = '',
}) => <div className={`shell ${className}`}>{children}</div>;

/** The product's mark. A monogram, matching the dashboard's header. */
export const Mark: React.FC<{ size?: number }> = ({ size = 26 }) => (
  <span
    aria-hidden="true"
    className="inline-flex items-center justify-center rounded-sm bg-accent-primary font-semibold text-on-accent"
    style={{ width: size, height: size, fontSize: size * 0.54 }}
  >
    D
  </span>
);

type ButtonProps = {
  href: string;
  variant?: 'primary' | 'secondary' | 'ghost';
  size?: 'md' | 'lg';
  children: React.ReactNode;
  external?: boolean;
};

/* A link, always. Every button on this site navigates somewhere, and a <button>
   that only exists to run a click handler is unreachable to anything that reads
   the page as links — including the keyboard user who wants to open the source
   in a new tab. */
export const LinkButton: React.FC<ButtonProps> = ({
  href,
  variant = 'secondary',
  size = 'md',
  children,
  external = false,
}) => {
  const base =
    'inline-flex items-center justify-center gap-2 rounded-md font-medium transition-colors duration-150 whitespace-nowrap';
  const sizes = size === 'lg' ? 'px-5 py-3 text-md' : 'px-4 py-2 text-sm';
  const variants = {
    primary:
      'bg-accent-primary text-on-accent hover:brightness-110 border border-transparent',
    secondary:
      'bg-bg-card text-text-main border border-border-strong hover:bg-bg-hover',
    ghost: 'text-text-secondary hover:text-text-main border border-transparent',
  }[variant];

  return (
    <a
      href={href}
      className={`${base} ${sizes} ${variants}`}
      {...(external ? { target: '_blank', rel: 'noreferrer noopener' } : {})}
    >
      {children}
    </a>
  );
};

/** A small labelled pill, for statuses and facts that need to look like one. */
export const Badge: React.FC<{ children: React.ReactNode; tone?: 'neutral' | 'info' }> = ({
  children,
  tone = 'neutral',
}) => (
  <span
    className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium ${
      tone === 'info'
        ? 'border-border-color bg-bg-card text-text-secondary'
        : 'border-border-color bg-bg-card text-text-muted'
    }`}
  >
    {children}
  </span>
);

export const Eyebrow: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div className="eyebrow">{children}</div>
);

export const SectionHead: React.FC<{
  eyebrow: string;
  title: string;
  lead?: string;
  id?: string;
}> = ({ eyebrow, title, lead, id }) => (
  <div id={id} className="max-w-2xl">
    <Eyebrow>{eyebrow}</Eyebrow>
    <h2 className="mt-3 text-2xl font-semibold tracking-tight text-text-main md:text-3xl">
      {title}
    </h2>
    {lead && <p className="mt-4 text-md text-text-secondary">{lead}</p>}
  </div>
);

/** A section with the page's standard vertical rhythm.
 *
 *  `id` is the anchor target the nav and the footer link to, so it is required
 *  to be able to be passed rather than being decoration on the element. */
export const Section: React.FC<{
  children: React.ReactNode;
  className?: string;
  rule?: boolean;
  id?: string;
}> = ({ children, className = '', rule = true, id }) => (
  <section id={id} className={`${rule ? 'rule' : ''} py-16 md:py-24 ${className}`}>
    <Shell>{children}</Shell>
  </section>
);

/* The theme control.

   It shows the theme you would switch TO, not the one you are in: the icon is
   the affordance, and a moon on a light page reads as "make it dark" without a
   legend. Same behaviour as the dashboard's, because it is the same control. */
export const ThemeToggle: React.FC = () => {
  const [theme, setTheme] = useState<Theme>(() =>
    typeof document === 'undefined' ? 'light' : currentTheme()
  );

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const toggle = useCallback(() => setTheme((t) => (t === 'dark' ? 'light' : 'dark')), []);
  const target = theme === 'dark' ? 'light' : 'dark';

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={`Switch to ${target} theme`}
      title={`Switch to ${target} theme`}
      className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border-strong bg-bg-card text-text-secondary transition-colors duration-150 hover:bg-bg-hover hover:text-text-main"
    >
      {theme === 'dark' ? (
        /* Sun: what you get by pressing it. */
        <svg viewBox="0 0 24 24" width="17" height="17" aria-hidden="true" focusable="false">
          <circle cx="12" cy="12" r="4" fill="none" stroke="currentColor" strokeWidth="2" />
          <path
            d="M12 2v2m0 16v2M4.93 4.93l1.41 1.41m11.32 11.32 1.41 1.41M2 12h2m16 0h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
          />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" width="17" height="17" aria-hidden="true" focusable="false">
          <path
            d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )}
    </button>
  );
};
