/* Shared primitives for the React views.

   Six components had grown six private copies of the same class strings —
   the card surface fourteen times, the section title fourteen times, the
   primary button seven. Copies drift: the cards had already diverged into
   p-6/p-4 variants and the buttons into three different paddings, none of
   which was a decision anyone made. These are the same class strings, named
   once, so a change to the surface is one edit rather than fourteen.

   The class names are deliberately kept identical to the ones they replace,
   except where DESIGN.md mandates a value the copies had drifted off — see
   `Button`'s 6px radius.
*/
import React from 'react';

/* §4 Toasts: "Bottom-right, 380px, Panel Surface fill, severity-colored left
   accent." The shell already owns that container (web/index.html
   #toast-container, which carries role="status" aria-live="polite"), and the
   inline script exposes showToast(). The React views were calling the native
   alert() instead — a blocking browser modal that cannot be styled, cannot be
   announced by that live region, and on the settings panels was the entire
   response to pressing Save.

   Falls back to the console when no shell is mounted, which is the case for
   the standalone Vite dev entry points (main-history.tsx and friends). */
export function toast(title: string, message: string): void {
  const shell = window as unknown as {
    showToast?: (t: string, m: string) => void;
  };
  if (typeof shell.showToast === 'function') {
    shell.showToast(title, message);
    return;
  }
  console.warn(`[driftwood] ${title} — ${message}`);
}

/* -------------------------------------------------------------------------
   Icons

   These replace emoji (🔄 📥 🔒). An emoji renders in the platform's colour
   font, so it ignores `currentColor`, sits on a different optical baseline
   from the Geist label beside it, and reads as a different weight on every
   OS. §3 gives the product exactly two typefaces; a third arriving through
   the emoji font is not one of them. Geometry is inherited from currentColor
   so an icon takes the colour of whatever it labels.
   ---------------------------------------------------------------------- */

const stroke = {
  viewBox: '0 0 16 16',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.5,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  'aria-hidden': true,
  className: 'w-4 h-4 shrink-0',
};

export const RefreshIcon: React.FC = () => (
  <svg {...stroke}>
    <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" />
    <path d="M13.5 2.5V5H11" />
  </svg>
);

export const DownloadIcon: React.FC = () => (
  <svg {...stroke}>
    <path d="M8 1.75v8.5" />
    <path d="M4.75 7 8 10.25 11.25 7" />
    <path d="M2.25 12.75h11.5" />
  </svg>
);

export const LockIcon: React.FC<{ open?: boolean }> = ({ open = false }) => (
  <svg {...stroke}>
    <rect x="3.25" y="7" width="9.5" height="6.25" rx="1" />
    <path d={open ? 'M5.75 7V5.25a2.25 2.25 0 0 1 4.4-.7' : 'M5.75 7V5.5a2.25 2.25 0 0 1 4.5 0V7'} />
  </svg>
);

/* -------------------------------------------------------------------------
   Surfaces
   ---------------------------------------------------------------------- */

type Pad = 'md' | 'sm';

export const Panel: React.FC<
  React.HTMLAttributes<HTMLDivElement> & {
    pad?: Pad;
    tone?: 'default' | 'error';
  }
> = ({ pad = 'md', tone = 'default', className = '', children, ...rest }) => (
  <div
    className={[
      'bg-bg-card rounded-xl border',
      pad === 'sm' ? 'p-4' : 'p-6',
      tone === 'error' ? 'border-accent-breaking' : 'border-border-color',
      className,
    ]
      .filter(Boolean)
      .join(' ')}
    {...rest}
  >
    {children}
  </div>
);

/* An h3 inside a Panel, and an h4 inside a card within one. The heading level
   is a prop rather than a second component because the two differ only in
   level and size, and splitting them is how a document ends up jumping from
   h2 to h4. */
export const PanelTitle: React.FC<{
  level?: 3 | 4;
  size?: 'base' | 'lg';
  className?: string;
  children: React.ReactNode;
}> = ({ level = 3, size = 'lg', className = '', children }) => {
  const Tag = (level === 3 ? 'h3' : 'h4') as 'h3';
  return (
    <Tag
      className={[
        'font-semibold text-text-main',
        size === 'lg' ? 'text-xl' : '',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {children}
    </Tag>
  );
};

/* -------------------------------------------------------------------------
   Controls

   §4: "Buttons: Flat fill, 6px radius, 1px structural border. … Tactile
   press: translateY(1px) on :active, 100ms."

   The radius is the one place these deviate from the strings they replace.
   The copies were `rounded-lg`, which is --radius-lg, 12px — double the 6px
   §4 specifies and double the shell's own .btn. The border on `primary` is
   accent-on-accent: it costs 2px of width and nothing visually, which is what
   makes it "structural" rather than decoration.

   `primary` is the brand accent, not accent-healthy. Ink on the fill is
   `text-on-accent`, which is white in light mode and near-black in dark —
   the accents are dark on the light ground and bright on the dark one, so
   neither `text-white` nor `text-bg-main` is right in both.
   ---------------------------------------------------------------------- */

type Variant = 'primary' | 'secondary' | 'quiet';
/** The three paddings the copies had drifted into, named by size instead. */
type Size = 'sm' | 'md' | 'lg';

const VARIANT: Record<Variant, string> = {
  primary:
    'border-accent-primary bg-accent-primary text-on-accent hover:bg-accent-primary/90',
  secondary: 'border-border-color text-text-main hover:bg-bg-hover',
  quiet: 'border-transparent text-text-muted hover:bg-bg-hover hover:text-text-main',
};

const SIZE: Record<Size, string> = {
  lg: 'px-6 py-3',
  md: 'px-4 py-2',
  sm: 'px-3 py-1.5',
};

export const Button: React.FC<
  React.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: Variant;
    size?: Size;
    /** Holds the button and says so, rather than silently doing nothing. */
    busy?: boolean;
  }
> = ({
  variant = 'secondary',
  size = 'lg',
  busy = false,
  className = '',
  disabled,
  children,
  ...rest
}) => (
  <button
    type="button"
    disabled={disabled || busy}
    aria-busy={busy || undefined}
    className={[
      // inline-flex + gap, matching the shell's own .btn. Without it a button
      // lays its label and any icon out as inline content, so "Refresh
      // History" broke onto two lines the moment it gained an icon.
      'inline-flex items-center justify-center gap-2 whitespace-nowrap',
      'rounded-sm border transition-all duration-100 active:translate-y-px',
      'disabled:opacity-50 disabled:cursor-not-allowed disabled:active:translate-y-0',
      SIZE[size],
      VARIANT[variant],
      className,
    ]
      .filter(Boolean)
      .join(' ')}
    {...rest}
  >
    {busy ? 'Working…' : children}
  </button>
);

/* A text field.

   The class string below had been copy-pasted six times, and every copy was
   wrong in the same two ways — which is the drift this file exists to end:

   - `border-border-color` is the hairline divider, and `--color-border-input`
     says in its own comment why a control cannot use it: WCAG 1.4.11 wants 3:1
     for the visual information that *identifies* a control, and a field whose
     only boundary is its border has nothing else, while "a card or a table can
     keep the hairline: its boundary is not what identifies it". Until this
     component nothing in the product referenced that token at all.
   - `focus:border-accent-healthy` is a severity token. §2 and DESIGN.md's ban
     list make healthy/amber/red mean contract state and nothing else, so a field
     that turns green on focus is asserting the contract is fine. The interactive
     accent is `--accent-primary`, which is what the shell's own
     `.form-group input:focus` (web/shell.css) already uses.

   `inputClass` is exported because the same string is what a `<select>` in this
   product needs, and a second component wrapping the same four classes would be
   a second place for them to drift from these. */
export const inputClass =
  'w-full px-4 py-3 rounded-sm border border-border-input bg-bg-hover text-text-main placeholder:text-text-muted focus:outline-none focus:border-accent-primary';

export const Input: React.FC<React.InputHTMLAttributes<HTMLInputElement>> = ({
  className = '',
  ...rest
}) => <input className={[inputClass, className].filter(Boolean).join(' ')} {...rest} />;

/* A labelled form control. §4: "Label above input … Settings panel
   max-width 550px." */
export const Field: React.FC<{
  label: string;
  children: React.ReactNode;
}> = ({ label, children }) => (
  <div className="space-y-4">
    <label className="block text-sm font-medium text-text-muted mb-2">
      {label}
    </label>
    {children}
  </div>
);

/* -------------------------------------------------------------------------
   View scaffolding
   ---------------------------------------------------------------------- */

/* The h2 every view opens with. `align` is the two shapes that existed:
   centred while the view has nothing to show, and split once it has an action
   to sit beside the title. */

/* The split shape cannot hold a title, an action and the gap between them in
   the narrowest tier. Measured at 320px: the row has 272px, the title has
   already been squeezed to its min-content floor of 91px — the width of
   "Version" alone at 24px, which nothing can shrink without breaking the word —
   and the action is 187px of a Button, `whitespace-nowrap` deliberately (see
   Button above: the label broke onto two lines the moment it gained an icon).
   91 + 187 = 278, so the row overflows itself by exactly 6px and the first
   width that fits is 326px.

   Six pixels is enough to be visible rather than theoretical: the button's left
   edge lands on top of the title's last line, because `items-center` holds it
   against a title that has wrapped to three lines of 24px text.

   Scoped to the shell's own narrowest tier rather than to the 326px where the
   overflow stops, matching the restacking the React islands already do at this
   tier. Worth it here on the merits, not just for consistency: below 479px the
   title is already breaking to two and three lines beside a vertically centred
   button — 400px currently renders a 96px-tall header reading "Version /
   History / Browser" with the button floating next to it — so stacking is
   better at those widths than what it replaces, not a cost paid to fix 320px.

   One pixel of imprecision, shared with every `max-[479px]:` in the components:
   Tailwind compiles that variant to `@media not all and (width >= 479px)`,
   which excludes 479px, where the shell's own `@media (max-width: 479px)`
   includes it. Measured, the row stacks at 326-478px and not at 479px. Nothing
   is broken at 479 — the row fits with 0 excess — so this is left as the
   convention rather than special-cased. */
export const ViewHeader: React.FC<{
  title: string;
  align?: 'center' | 'between';
  action?: React.ReactNode;
  lead?: string;
}> = ({ title, align = 'between', action, lead }) => (
  <div
    className={
      align === 'center'
        ? 'text-center py-12'
        : 'flex justify-between items-center gap-y-3 max-[479px]:flex-col max-[479px]:items-start'
    }
  >
    <h2
      className={
        align === 'center'
          ? 'text-2xl font-semibold tracking-tight text-text-main mb-4'
          : 'text-2xl font-semibold tracking-tight text-text-main'
      }
    >
      {title}
    </h2>
    {lead && <p className="text-text-muted max-w-xl mx-auto">{lead}</p>}
    {action}
  </div>
);

/* §4: "Empty states: Instrument-themed copy that directs action … with the
   trigger button referenced by name. Never bare 'No data.'"

   The ○ is §4's no-baseline shape, which is exactly what an empty history
   is: nothing has been observed yet. */
export const EmptyState: React.FC<{
  title: string;
  body: React.ReactNode;
  action?: React.ReactNode;
}> = ({ title, body, action }) => (
  <Panel className="text-center">
    <div className="text-text-muted text-2xl font-semibold" aria-hidden="true">
      ○
    </div>
    <h3 className="font-semibold text-text-main mt-2 mb-1">{title}</h3>
    <p className="text-text-muted text-sm max-w-xl mx-auto">{body}</p>
    {action && <div className="flex justify-center mt-4">{action}</div>}
  </Panel>
);

/* §4: "Loaders: Skeleton rows matching table dimensions — shimmering Hairline
   bars. No circular spinners anywhere."

   This replaced an `animate-pulse` circle, which is the circular spinner §4
   bans. The classes live in web/shell.css (.skeleton-rows/.skeleton-row) —
   same stylesheet, so the shimmer is defined once for both the shell and the
   React views. */
export const SkeletonRows: React.FC<{ rows?: number }> = ({ rows = 4 }) => (
  <div className="skeleton-rows" aria-hidden="true">
    {Array.from({ length: rows }, (_, i) => (
      <div className="skeleton-row" key={i} />
    ))}
  </div>
);
