# Design System: Driftwood — API Contract Drift Monitor

> **Section numbers are a stable interface.** `web/shell.css`, `web/index.html` and
> `frontend-react/src/` cite this document by section (`§2`, `§3`, `§4`, `§5`) in comments
> that explain *why* a value is what it is. Renumbering breaks those references. Add and
> amend freely; do not reorder.

## 1. Visual Theme & Atmosphere

A clinical lab monitor rendered as a developer tool — **light by default, dark on request.**
The atmosphere is calm, precise, and instrument-like: a well-lit observability console in a
quiet room, where a single red indicator carries real weight because nothing else shouts.
Density is cockpit-leaning (7/10): compact rows, tight metadata, numbers always in mono.
Variance is restrained (4/10): the layout is offset and asymmetric in the vitals strip and
drawer, but data tables stay strictly grid-aligned for scanability. Motion is fluid and
purposeful (5/10): spring micro-interactions confirm state changes — a new breaking alert
*moves* — but nothing loops for decoration except the live SSE pulse, which represents a
real connection.

**Both themes are the same design, not two designs.** They share one token list, one set of
names, and one set of components; only the values differ. A change that has to be made twice
is a change that was made wrong — see §2.

**The traffic table's row sparkline** is the signature element, and it earns that by being
computed rather than decorative: it plots each endpoint's own request durations across the
window, and the line turns from Data Sky to Caution Amber when the newest observation sits
well above that endpoint's own median. It is a statement about the data, not a repeat of the
Contract column's colour beside it.

**The contract-stability sparkline** is the second signature element, and it is built. It
plots contract match health across the endpoint's last ~50 observations in the Version
History panel, flat and high while the contract holds, cliffing at the moment it broke —
the one thing a single "current stability" percentage cannot show, because it has no memory
of when things changed. The line is the **running** match rate rather than a per-request
pass/fail: a per-request series over a healthy endpoint is a solid block of one value and
reads as a filled rectangle, while the running rate starts where the first observation put
it and settles as evidence accumulates, so the shape carries the trend.

Two rules keep the number beside it honest, and both are the reason it is drawn from the
observation window rather than from a version list. An observation with no baseline to be
measured against is excluded from the denominator rather than counted as a failure — an
endpoint that has been seen but never locked cannot fail to match a contract it does not
have, and folding those in would report a brand-new endpoint as 0% stable. And the panel
states how many requests the figure covers, because a rate over three sightings and a rate
over fifty are not the same claim.

This element has been built wrong once already: an earlier version averaged `Math.random()`
per version and printed it to one decimal under the caption "Percentage of requests matching
the baseline contract" — a number none of that arithmetic computed, that changed on every
render, and that coloured itself green or red as a verdict. §7's ban on invented numbers
applies to this column with full force, as it does everywhere else.

## 2. Color Palette & Roles

Two palettes, one token list. Every colour is declared as a literal exactly **once per
palette** and referenced by name everywhere else; `frontend-react/src/tokens.css` is the single
definition site for both, and `web/shell.css` consumes it through the shell aliases. Adding a
colour means one literal in the light block, one in the dark block, and one alias line — never
a third copy of a hex value anywhere.

`tokens.css` is a file of its own rather than a block in `index.css` because a second surface
reads it — the marketing site in §8 imports it directly, and the alternative was the site
carrying its own copy of the palette, which is the drift this rule exists to prevent.
`index.css` holds the layer order and the imports and is otherwise about the dashboard; the
tokens are the part the two surfaces share.

**Light** is the default, and is the world the product is designed in. **Dark** is the palette
this dashboard originally shipped, kept because it was good. Precedence: an explicit
`data-theme` on `<html>` wins; with no stored choice, the OS preference decides. `data-theme`
must be set on `<html>` and not `<body>` — a custom property's `var()` is substituted on the
element that *declares* it, so a value declared on `:root` reads too late from `<body>`.

### Light

| Role | Value | Contrast | Note |
|---|---|---|---|
| page / card / inset | `#FAFAFA` / `#FFFFFF` / `#F4F4F4` | — | page, card, and hover well |
| surface-3 | `#EEEEEE` | — | table headers, search bar, recessed strips |
| hairline / strong border | `#E6E6E6` / `#BDBDBD` | — | divider vs. hover and emphasis edge |
| **input border** | `#848484` | 3.7 on `#FFF`, 3.2 on `#EEE` | see below — not the hairline |
| text primary / secondary / muted | `#1A1A1A` / `#605F5F` / `#6C6C6C` | 16.7 / 6.1 / 5.0 | measured on `#FAFAFA` |
| **accent** (interactive, brand) | `#1A6FD4` | 4.7 | buttons, links, active nav, focus ring |
| healthy / warning / breaking | `#0A6B45` / `#7A5000` / `#C4001A` | 6.3 / 6.8 / 6.0 | contract state only |
| info | `#0B5FB8` | 6.0 | INFO deltas, GET |
| on-accent | `#FFFFFF` | — | ink *on* an accent fill |
| scrim | `rgba(16,24,40,.45)` | — | modal and guided-tour veil |

### Dark

| Role | Value | Contrast | Note |
|---|---|---|---|
| page / card / hover | `#09090B` / `#101014` / `#18181D` | — | never pure `#000000` |
| surface-3 | `#232329` | — | |
| hairline / strong border | `#1C1C1E` / `#3A3A40` | — | |
| **input border** | `#757575` | 4.1 on `#101014`, 3.4 on `#232329` | |
| text primary / secondary / muted | `#E4E4E7` / `#B4B4BB` / `#8E8E93` | 15.7 / 9.7 / 6.1 | |
| **accent** (interactive, brand) | `#4096FF` | 6.7 | |
| healthy / warning / breaking | `#00C9A7` / `#FF9F0A` / `#FF3B30` | 9.4 / 9.7 / 5.6 | |
| info | `#5AC8FA` | 10.5 | |
| on-accent | `#09090B` | — | |
| scrim | `rgba(0,0,0,.6)` | — | |

### Why the palette is shaped this way

**The brand accent is not the healthy accent.** They were one value once, which meant a
primary button and a "this contract still matches its baseline" badge were the same green —
two unrelated statements wearing one signal. `accent-primary` is now the interactive/brand
colour and `accent-healthy` is a contract state and nothing else. On-accent ink follows the
fill rather than being white: light-mode accents are dark (white ink), dark-mode accents are
bright (near-black ink), so neither `#FFFFFF` nor `#09090B` is right in both themes.

**Input borders are their own token.** WCAG 1.4.11 requires 3:1 for the visual information
that *identifies* a control, and a text field whose only boundary is its border has nothing
else. `#E6E6E6` on `#FFFFFF` is 1.2:1 — a hairline divider, not a control edge. Cards and
tables keep the hairline, because their boundary is not what identifies them. The input value
is measured against the control's **own fill**, not the page, since the search bar sits on
Surface-3 and settings inputs on Raised Hover.

**Contrast is measured, never assumed.** The reference system this palette derives from uses
`#4096FF` on `#FAFAFA`, which is **2.86:1** — below the 3:1 floor for UI and far below 4.5:1
for text — so the light accent is darkened to `#1A6FD4`. Its `#999` muted text is 2.73:1 and
is not usable as text at all. The dark values needed no such correction: `#4096FF` on
`#09090B` is 6.7:1. Every value in the tables above was verified against the shipped
stylesheet in a rendered DOM, in both themes, at both desktop and mobile widths.

**Tints are tuned per surface, not fixed.** A 15% tint that reads as a whisper on a dark
ground *stains* a light one, and several tint levels had to be lowered: method chips to 12%,
the active nav item to 5%, the `.btn-danger` hover deepened its border instead of raising its
fill. Status badges keep 15% fill over a 30% border.

**Roles are semantic and exclusive** — healthy/warning/breaking/info mean contract state and
nothing else. Method badges are a deliberate, documented exception: they borrow the same four
hues to code the *verb* (GET = info, POST = healthy, PUT = warning, DELETE = breaking), which
is the convention developers already read. The collision is resolved by context and by never
leaving it to colour alone — a method chip is always uppercase mono and never appears in a
position where a contract badge could be, and the brand accent is never used for one, so the
interactive colour stays unambiguous.

## 3. Typography Rules

- **Display:** `Geist` (500–600) — Brand wordmark, view titles, vital values. Track-tight (-0.02em), hierarchy through weight and color, not size jumps. Max 1.5rem for values, 1.25rem for brand.
- **Body:** `Geist` (400–500) — Table content, alert messages, settings copy. Relaxed 1.5 leading, 65ch max measure.
- **Mono:** `Geist Mono` (400–600) — All numbers (durations, request rates, confidence scores), API paths, JSON payloads, method badges, timestamps. Density-7 rule: every metric is mono.
- **Banned:** `Inter` (default AI dashboard font), all serif fonts (dashboards are sans-only), system-ui fallbacks as primary choices.

`Geist Mono`, not `JetBrains Mono`: the shell sets mono at weights **400 and 500** (table
headers and metric values), and the mono chosen has to ship both. Geist Mono serves
400/500/600 and shares a lineage with the sans, which is what the reference does. Fonts are
loaded from Google Fonts by `web/index.html`, and `--font-sans` / `--font-mono` are set
explicitly in `@theme` — without them `font-sans` falls back to preflight's system stack and
every rule below is silently inert.

## 4. Component Stylings

* **Buttons:** Flat fill, 6px radius (`--radius-sm`), 1px structural border. Primary = `accent-primary` fill with `on-accent` ink — *not* accent-healthy, which is a contract state (§2). Tactile press: `translateY(1px)` on `:active`, 100ms. No glows, no gradients, no icons-only ambiguity — every button has a text label.
* **Status badges:** Pill shape (9999px radius), 1px tinted border at 30% alpha, background at 15% alpha of the semantic color. Shape + color dual coding for colorblind safety: healthy = ●, warning = ▲, breaking = ■, no-baseline = ○. Breaking badge pulses once on arrival, then rests — no infinite flashing.
* **Method badges:** Mono uppercase, 4px radius, tinted at 12% by verb (GET = Data Sky, POST = healthy, PUT = Caution Amber, DELETE = Fault Red — see §2 on this exception). Compact: 0.2rem × 0.5rem padding.
* **Vitals strip:** Three instrument cells — Request Rate, Error Rate, Contract Health — laid out offset-asymmetric (health ring larger, right-aligned cluster). The rate "sparklines" are 20px recessed tracks filled by a two-stop `linear-gradient` to a percentage; the class is named `.vital-sparkline` for its slot in the layout, but it is a bar, not a trend line. The contract health ring is a `conic-gradient` donut, 60px, 3px stroke, healthy sweep over the Hairline track, and it re-colours to warning or breaking as health falls.
* **Traffic table:** Full-bleed within its card, 0.75rem row padding, Hairline row rules only (no vertical gridlines). Row hover lifts background to Raised Hover. Entire row is the click target opening the detail drawer. The row sparkline plots request duration per §1; the Contract cell carries the badge.
* **Detail drawer:** 720px right-side sheet, slides with spring physics (`stiffness: 100, damping: 20`). Contains diff summary card (severity-tinted left border, 4px) and observed payload in a mono code box (max-height 280px, internal scroll).
* **Deltas (diff items):** Left-border severity bar (4px), background at 10% alpha of severity color, mono JSON path in Data Sky, message in Primary Ink.
* **Toasts:** Bottom-right, 380px, Panel Surface fill, severity-colored left accent. Slide-in 0.3s ease, auto-dismiss 5s. Breaking toasts carry the Fault Red accent.
* **Overlays:** Modal and guided-tour veils paint `--scrim`, which is dark in **both** themes. They previously painted the page background at 70–80%, which reads as a veil only because the dark page is dark; on a light page the same rule lays a white wash over a white page and the dialog stops separating from what is behind it.
* **Inputs:** Label above input, mono placeholder, focus ring is 1px accent border swap — no box-shadow ring. Border is `--border-input`, not the hairline (§2). Settings panel max-width 550px.
* **Loaders:** Skeleton rows matching table dimensions — shimmering Hairline bars. No circular spinners anywhere.
* **Empty states:** Instrument-themed copy that directs action: "Listening for HTTP network traffic…" with the trigger button referenced by name. Never bare "No data."

## 5. Layout Principles

- App shell: sticky 58px header, full-width vitals strip below it, then a 280px sidebar + fluid main grid. Sidebar collapses to icon rail below 1024px, hides entirely below 768px (nav moves to a top dropdown).
- **Header degrades in a measured order below 768px**, because it overflows otherwise: the wordmark is clipped first (the monogram carries the identity and the page title is in the tab — clipped, not `display: none`, so it stays in the accessibility tree), then the wide CTA label below 480px. Every step is a measurement, not a guess.
- Content containment: main views padded 1.5rem, no max-width cap — this is a cockpit, tables deserve the full window.
- The vitals strip is the only asymmetric element: 3 cells with the ring offset right. Everything below it is strictly grid-aligned.
- No overlapping elements, ever. No absolutely-positioned content stacking. Drawer and toasts are the only overlays, and both are z-managed (100/200).
- No 3-equal-card feature rows. Baselines and history render as stacked full-width entries — this is data, not marketing.
- Grid over flexbox math; no `calc()` percentage hacks. Fixed sidebar column, fluid main column.
- Table columns: Method (fixed 90px), Path (fluid), Status (fixed 80px), Duration (fixed 100px), Contract (fixed 160px), Sparkline (fixed 120px), Time (fixed 100px). Mono throughout. The table has a 720px floor and scrolls inside its own container below it; the page body never scrolls horizontally, at any width.

## 6. Motion & Interaction

- Spring physics for drawer and toasts: `stiffness: 100, damping: 20` (≈250ms settle). No linear easing on anything interactive.
- SSE pulse: the 8px status dot breathes at 2s opacity cycle — the only perpetual animation, and only while the event stream is open. On disconnect it stops pulsing and turns Caution Amber.
- New traffic rows cascade in with a 40ms stagger, translateY(4px) → 0 + opacity. Breaking rows arrive with a single Fault Red flash (300ms), then rest at the static badge.
- Sparklines redraw on each new observation with a 150ms transform transition — no layout thrash, `transform`/`opacity` only.
- **Theme switching is instant and unanimated.** There is no cross-fade between light and dark: a transition on a theme swap animates every property on every element at once, and the toggle is a preference, not an event worth choreographing. The pre-paint script in `<head>` sets the attribute before first paint, so there is no flash of the wrong theme.
- All animation honors `prefers-reduced-motion`: pulses, cascades, and slides collapse to instant state changes. Smooth anchor scrolling is likewise opt-in via `no-preference`, because a long smooth scroll is exactly what that setting exists to suppress.

## 7. Anti-Patterns (Banned)

This list binds **both surfaces** — the dashboard and the marketing site (§8). It was written
for the dashboard, and the parenthetical on the card-grid rule ("this is data, not marketing")
reads as though a marketing page might be exempt. It is not. The site is where a reader decides
whether the product is serious, so it is the last place to make an exception, and the card-grid
ban in particular caught a real violation there: a three-step "how it works" section shipped as
three equal cards, which is both the banned shape and a lie about content that is a sequence.

- No emojis in UI chrome (severity icons are geometric shapes: ● ▲ ■ ○)
- No `Inter` font — `Geist` only; no serifs anywhere (both surfaces; the site's display sizes
  are additional tokens, not a second typeface — see §8)
- No pure black `#000000` — dark mode's floor is Deep Monitor `#09090B`; light mode's ground is `#FAFAFA` and its cards are `#FFFFFF`
- No colour written as a literal outside the two palette blocks in `tokens.css` — one definition site, or the themes drift apart
- No neon/outer glow shadows, no purple/violet anywhere
- No gradient text on headers — the brand wordmark is solid Primary Ink
- No custom mouse cursors
- No overlapping or absolutely-stacked content
- No 3-column equal card grids
- No AI copywriting clichés ("Elevate", "Seamless", "Unleash") — copy states what the tool observes
- No filler UI text ("Scroll to explore", bouncing chevrons)
- No fake precision numbers — rates and percentages come from real counts
- No decorative use of severity colors — healthy/amber/red mean contract state, nothing else (§2's method-badge exception is the only one, and it is labelled)
- No infinite alert flashing — one pulse on arrival, then rest
- No class name that lies about what an element is or does; if the shape changes, rename it

## 8. The Marketing Site

A second surface, at `site/`, deployed separately from the binary. It exists because the two
have different jobs and different audiences: the dashboard is **Operate** — someone is reading
a live traffic table and wants the breaking row in under a second — while the landing page is
**Persuade** and the try-it-out page is **Operate on a demo**, in a browser, for someone who has
not installed anything.

**It shares §2's tokens and nothing else.** `site/src/styles.css` imports
`frontend-react/src/tokens.css` — the same file the dashboard's `index.css` imports — and writes
no colour, radius, shadow or font of its own. That is the whole mechanism by which the two read
as one product, and it is why the token file was split out of `index.css` in the first place: the
dashboard's *shell* (traffic table, drawer, toasts, wizard) must not come along with its palette.
The site adds exactly two tokens, `--text-display` and `--text-display-lg`, as **new names**
rather than by redefining the shared scale — overriding `--text-4xl` would give that token two
meanings depending on which page you are on.

What it does not share is the shell. A marketing page has no use for a traffic table.

**The theme is one setting across both.** Same `data-theme` attribute on `<html>`, same
`driftwood-theme` localStorage key, same pre-paint inline script in `<head>`. A reader who chose
dark in the app is not flashed light when they land on the site, and their choice carries back.
The key being identical is load-bearing and worth stating: a second key would look like it
worked while silently disagreeing with the other page.

Tailwind's `dark:` variant is **unusable** on either surface. It follows `prefers-color-scheme`,
so it would ignore a reader who has explicitly chosen light on a dark machine — the one case the
token layer goes out of its way to honour. The site swaps its two dashboard screenshots by theme
with `.theme-light-only` / `.theme-dark-only` in `styles.css`, mirroring the token layer's exact
conditions including the `:not([data-theme="light"])` guard.

**Copy rules.** Every claim on the site must be true of this codebase — the same rule §7 already
applies to numbers, extended to prose. No "trusted by" strip, no invented logos, no customer
quotes, no pricing table: there are no customers to name and nothing to sell, so those slots hold
a facts strip (each figure checkable in the repository) and an FAQ that answers the unflattering
questions — "is it safe to bind to my network", "what does it not do" — rather than only the easy
ones. Terminal output on the page is captured from a real run and the caption says what was
changed.

**The try-it-out page must never fall back to canned data.** It drives a real instance over
the control API, same-origin, because the control plane's CORS allowlist is localhost-only
(`internal/server/server.go`, `isAllowedOrigin`) — see `Caddyfile.driftwood`. If it cannot reach
an instance it says so and shows the command to start one. A demo that quietly substitutes
pre-recorded output is worse than no demo, because the reader takes away a belief that nothing
they did established. Its liveness probe validates that the reply *parses as JSON*, not that the
status was 200: a static host with an SPA fallback answers an unknown API path with 200 and the
site's own HTML, which is the same failure this project shipped once in the dashboard.
