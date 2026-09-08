# Design System: Driftwood — API Contract Drift Monitor

## 1. Visual Theme & Atmosphere

A clinical lab monitor rendered as a developer tool. The atmosphere is calm, precise, and instrument-like — think a well-lit observability console in a quiet room, where a single red indicator carries real weight because nothing else shouts. Density is cockpit-leaning (7/10): compact rows, tight metadata, numbers always in mono. Variance is restrained (4/10): the layout is offset and asymmetric in the vitals strip and drawer, but data tables stay strictly grid-aligned for scanability. Motion is fluid and purposeful (5/10): spring micro-interactions confirm state changes — a new breaking alert *moves* — but nothing loops for decoration except the live SSE pulse, which represents a real connection.

The one signature element: a **contract stability sparkline** per endpoint row — a 20px-tall trend line showing contract match health over the last 50 observations. Flat and high = stable; a cliff drop = the moment a contract broke. This is the element Driftwood is remembered by; everything around it stays quiet.

## 2. Color Palette & Roles

- **Deep Monitor** (`#09090B`) — Primary background surface. Zinc-950 off-black; never pure `#000000`.
- **Panel Surface** (`#101014`) — Cards, drawer, sidebar fill. One step lighter than Deep Monitor.
- **Raised Hover** (`#18181D`) — Hover states, recessed input wells, sparkline track.
- **Primary Ink** (`#E4E4E7`) — Primary text, table cell content. Zinc-200.
- **Muted Telemetry** (`#8E8E93`) — Secondary text: timestamps, durations, metadata, labels.
- **Hairline Structure** (`#1C1C1E`) — 1px borders, dividers, table rules. Structural, not decorative.
- **Vital Teal** (`#00C9A7`) — The single accent. Healthy contract state, SSE-connected pulse, focus rings, active nav, primary CTA fill. Saturation held at ~60% — never neon.
- **Caution Amber** (`#FF9F0A`) — WARNING severity deltas, warning badges, non-breaking drift markers. Functional only, never decorative.
- **Fault Red** (`#FF3B30`) — BREAKING severity, alert counters, breaking toasts. Appears only when a contract actually broke.
- **Data Sky** (`#5AC8FA`) — INFO-level deltas, GET method badge, informational links. Lowest-salience semantic hue.

Roles are semantic and exclusive: teal = healthy, amber = warning, red = breaking, sky = info. These colors never appear outside their meaning.

## 3. Typography Rules

- **Display:** `Geist` (500–600 weight) — Brand wordmark, view titles, vital values. Track-tight (-0.02em), hierarchy through weight and color, not size jumps. Max 1.5rem for values, 1.25rem for brand.
- **Body:** `Geist` (400–500) — Table content, alert messages, settings copy. Relaxed 1.5 leading, 65ch max measure.
- **Mono:** `JetBrains Mono` (400–500) — All numbers (durations, request rates, confidence scores), API paths, JSON payloads, method badges, timestamps. Density-7 rule: every metric is mono.
- **Banned:** `Inter` (default AI dashboard font), all serif fonts (dashboards are sans-only), system-ui fallbacks as primary choices.

## 4. Component Stylings

* **Buttons:** Flat fill, 6px radius, 1px structural border. Primary = Vital Teal fill on dark text where contrast allows, else solid teal with near-black label. Tactile press: `translateY(1px)` on `:active`, 100ms. No glows, no gradients, no icons-only ambiguity — every button has a text label.
* **Status badges:** Pill shape (9999px radius), 1px tinted border at 30% alpha, background at 15% alpha of the semantic color. Shape + color dual coding for colorblind safety: healthy = ●, warning = ▲, breaking = ■, no-baseline = ○. Breaking badge pulses once on arrival, then rests — no infinite flashing.
* **Method badges:** Mono uppercase, 4px radius, tinted by verb (GET = Data Sky, POST = Vital Teal, PUT = Caution Amber, DELETE = Fault Red). Compact: 0.2rem × 0.5rem padding.
* **Vitals strip:** Three instrument cells — Request Rate, Error Rate, Contract Health — laid out offset-asymmetric (health ring larger, right-aligned cluster). Sparklines are 20px-tall tracks with a semantic gradient fill. The contract health ring is a `conic-gradient` donut, 60px, 3px stroke, teal sweep over Hairline track.
* **Traffic table:** Full-bleed within its card, 0.75rem row padding, Hairline row rules only (no vertical gridlines). Row hover lifts background to Raised Hover. Entire row is the click target opening the detail drawer.
* **Detail drawer:** 720px right-side sheet, slides with spring physics (`stiffness: 100, damping: 20`). Contains diff summary card (severity-tinted left border, 4px) and observed payload in a mono code box (max-height 280px, internal scroll).
* **Deltas (diff items):** Left-border severity bar (4px), background at 10% alpha of severity color, mono JSON path in Data Sky, message in Primary Ink.
* **Toasts:** Bottom-right, 380px, Panel Surface fill, severity-colored left accent. Slide-in 0.3s ease, auto-dismiss 5s. Breaking toasts carry the Fault Red accent.
* **Inputs:** Label above input, mono placeholder, focus ring is 1px Vital Teal border swap — no box-shadow ring. Settings panel max-width 550px.
* **Loaders:** Skeleton rows matching table dimensions — shimmering Hairline bars. No circular spinners anywhere.
* **Empty states:** Instrument-themed copy that directs action: "Listening for HTTP network traffic…" with the trigger button referenced by name. Never bare "No data."

## 5. Layout Principles

- App shell: sticky 58px header, full-width vitals strip below it, then a 280px sidebar + fluid main grid. Sidebar collapses to icon rail below 1024px, hides entirely below 768px (nav moves to a top dropdown).
- Content containment: main views padded 1.5rem, no max-width cap — this is a cockpit, tables deserve the full window.
- The vitals strip is the only asymmetric element: 3 cells with the ring offset right. Everything below it is strictly grid-aligned.
- No overlapping elements, ever. No absolutely-positioned content stacking. Drawer and toasts are the only overlays, and both are z-managed (100/200).
- No 3-equal-card feature rows. Baselines and history render as stacked full-width entries — this is data, not marketing.
- Grid over flexbox math; no `calc()` percentage hacks. Fixed sidebar column, fluid main column.
- Table columns: Method (fixed 90px), Path (fluid), Status (fixed 80px), Duration (fixed 100px), Contract (fixed 160px), Sparkline (fixed 120px), Time (fixed 100px). Mono throughout.

## 6. Motion & Interaction

- Spring physics for drawer and toasts: `stiffness: 100, damping: 20` (≈250ms settle). No linear easing on anything interactive.
- SSE pulse: the 8px status dot breathes at 2s opacity cycle — the only perpetual animation, and only while the event stream is open. On disconnect it stops pulsing and turns Caution Amber.
- New traffic rows cascade in with a 40ms stagger, translateY(4px) → 0 + opacity. Breaking rows arrive with a single Fault Red flash (300ms), then rest at the static badge.
- Sparklines redraw on each new observation with a 150ms transform transition — no layout thrash, `transform`/`opacity` only.
- All animation honors `prefers-reduced-motion`: pulses, cascades, and slides collapse to instant state changes.

## 7. Anti-Patterns (Banned)

- No emojis in UI chrome (severity icons are geometric shapes: ● ▲ ■ ○)
- No `Inter` font — `Geist` only; no serifs anywhere (dashboard)
- No pure black `#000000` — Deep Monitor `#09090B` is the floor
- No neon/outer glow shadows, no purple/violet anywhere
- No gradient text on headers — the brand wordmark is solid Primary Ink
- No custom mouse cursors
- No overlapping or absolutely-stacked content
- No 3-column equal card grids
- No AI copywriting clichés ("Elevate", "Seamless", "Unleash") — copy states what the tool observes
- No filler UI text ("Scroll to explore", bouncing chevrons)
- No fake precision numbers — rates and percentages come from real counts
- No decorative use of severity colors — teal/amber/red mean contract state, nothing else
- No infinite alert flashing — one pulse on arrival, then rest
