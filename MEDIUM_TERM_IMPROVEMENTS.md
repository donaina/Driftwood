# Medium-Term Improvements for Driftwood

Following the completion of immediate wins (enhanced empty states, guided tour, improved simulator UX, contextual help, and share functionality), here are proposed medium-term improvements to make Driftwood more powerful and user-friendly.

## Proposed Improvements

### 1. Setup Wizard for First-Time Users ✓ COMPLETED
**Goal:** Guide new users through initial configuration to reduce setup friction.

**Status (2026-09-21).** Shipped as a three-step wizard — and shipping it meant first
removing two fabrications from its original implementation, both of which are worth
knowing about because they are the pattern this doc keeps running into:

- `detectLocalhost` "reported *Found API at http://localhost:3000* without opening a single
  socket" — it looped a port list and hardcoded a hit behind a 1s timer, so it announced a
  discovery it had not made and said the same thing on a machine with nothing running.
- Step 2 printed "Found 5 API endpoints" after a 1.5s timer regardless of what was behind
  the target. The honest answer at that point is usually zero, because Driftwood learns
  endpoints from traffic and a fresh install has none yet.

It now probes real ports and reports what it finds, including finding nothing.

**Two further deviations from the proposal below:** it is a **view, not a `/setup` route**,
and it opens when `/_driftwood/api/setup-state` reports that nobody has ever named a target
— *not* when no baselines exist. The baseline-count trigger was removed deliberately: every
request the proxy sees is auto-baselined, so a browser's incidental `GET /favicon.ico` was
enough to make a genuinely untouched install look configured. Counting baselines measured
traffic, not intent. There is also **no "Try with Demo" button**; the simulator panel is
where that lives.

**Features:**
- Interactive walkthrough for configuring target API
- Auto-detection of common local development servers (localhost:3000, localhost:8080, etc.)
- One-click baseline creation for detected APIs
- Explanation of what Driftwood monitors and how to interpret results
- Option to skip and configure manually

**Implementation:**
- New `/setup` route that shows on first visit if no baselines exist
- Uses same design system (Geist fonts, the accent-primary brand blue, etc.)
- Progress indicator with 3-4 steps
- "Try with Demo" button that pre-configures the mock simulator *(not built — the simulator
  panel serves this; see Status)*

### 2. Scenario Library — SHIPPED AS A CATALOGUE
**Goal:** Provide pre-configured API breaking change scenarios for learning and experimentation.

**Status (2026-09-21).** Shipped as a **catalogue**, not a runner: 11 scenarios
across REST, GraphQL and gRPC, filterable by severity *and* protocol. Three items
in the proposal below did not ship, and two of them should not:

- **One-click activation / the "Load Scenario" button — not built.** It drove
  `setMockMode`, which is private to the shell's closure and not on `window`, and
  no React component calls into the shell at all. Wiring it would mean building a
  new shell↔React bridge, which is a feature rather than a fix for a stale
  branch. Each scenario instead carries a caveat naming whether the built-in
  simulator can reproduce it, so the library is honest about being a reference.
- **Colour-coding difficulty — rejected, not deferred.** `DESIGN.md` §7 reserves
  healthy/warning/breaking/info for contract state and nothing else. Spending
  three of those four roles on *effort* means a Beginner scenario wearing Caution
  Amber reads as a warning about the contract. Difficulty is neutral
  `--text-muted` text in the shell's `.label-caps` register, and takes no shape
  either, since §4's ● ▲ ■ ○ alphabet is reserved the same way.
- **User-created scenario sharing — not built.** Nothing stores a scenario.

**Features:** the list below is the original proposal, kept for provenance.

- Collection of common API breaking changes (type changes, missing fields, status code changes, etc.)
- Each scenario includes:
  - Description of what changed
  - Expected impact on consumers
  - How to fix it
  - One-click activation *(not built — see Status)*
- Filterable by severity (breaking, warning, info) and type (REST, GraphQL, gRPC) *(shipped, both axes)*
- User-created scenario sharing *(not built)*

**Implementation:** annotated with what actually shipped, so nothing here is
copied back into the product as though it were a description of it.

- New `/scenarios` route in the dashboard — shipped, as the Scenario Library nav entry
- Scenario cards with Vital Teal/Fault Red/Caution Amber indicators — **rejected**;
  cards carry the §4 contract-state badge for severity and a neutral difficulty label
- "Load Scenario" button that configures mock simulator accordingly — **not built**
- Integration with guided tour for educational paths — not built

### 3. Contract Evolution Timeline — MOSTLY SHIPPED
**Goal:** Visualize how API contracts change over time to help teams understand drift patterns.

**Status (2026-09-21).** Shipped in `EndpointHistory.tsx`, inside the **History** view
(`nav-history` → `showHistoryLibrary`) rather than as a new tab in the baseline detail view:

- Timeline of baseline versions, each with its change description and a
  shape-and-colour status node (the §4 alphabet, not colour alone) — shipped
- Selecting two versions renders a **Version Comparison** panel — shipped
- Contract-stability sparkline across observed requests — shipped
- **PNG/SVG export — NOT shipped.** Both buttons are on screen, and `handleExportTimeline`
  (`HistoryView.tsx`) is a placeholder that toasts *"Export for X is planned for a future
  update."* It does not claim a file was written, which is what separates this from the
  deleted thresholds form — but the buttons themselves promise an artifact that does not
  exist, and they are the one soft spot in an otherwise honest view. Either wire them or take
  them off the screen.

**Features:**
- Timeline view showing baseline versions and when changes occurred
- Hover-over tooltips showing what changed in each version
- Ability to compare any two versions side-by-side
- Export timeline as PNG/SVG for reports
- Contract stability score over time (percentage of requests matching baseline)

**Implementation:**
- New tab in baseline/detail view
- Uses sparkline concept expanded to show historical versions
- Color-coded dots: ● Vital Teal (healthy), ■ Fault Red (breaking), ▲ Caution Amber (warning)
- Mono-spaced timestamps for consistency

### 4. Interactive Integration Guides — SHIPPED
**Goal:** Provide copy-paste ready integration examples for popular frameworks.

**Status (2026-09-21).** Shipped, as `IntegrationsLibrary.tsx` mounted by the
`nav-integrations` entry — all five frameworks (Express, FastAPI, NestJS, Django, Rails), each
with a code snippet and numbered setup steps. Deviations from the proposal:

- **"Try in Sandbox" — not built**, and correctly so: there is no sandbox to run anything in,
  so the button would have been a control reporting an action it did not take.
- **No copy button.** "Ready-to-copy" means the snippet is a selectable `<pre>`, not one
  click. Worth adding, and it is real work rather than the fake it would have been before.
- **No framework logos** — the card carries the framework's first letter in a tinted square.
- **One live palette breach**, to fold into the next UI pass: that square is
  `bg-accent-info/20` + `text-accent-info` (`IntegrationsLibrary.tsx:183-184`), which spends
  a contract-state role on chrome — the same mistake §1, §6 and §7 made in prose. It wants
  `accent-primary`. `EndpointHistory.tsx:269` does it too, on the pin button's pressed state.

**Features:**
- Framework-specific guides (Express, FastAPI, NestJS, Django, Rails, etc.)
- Ready-to-copy middleware/code snippets
- Explanation of where to place Driftwood in the stack
- Common pitfalls and troubleshooting tips
- "Try in Sandbox" button that shows live example

**Implementation:**
- New `/integrations` route
- Card-based layout with framework logos
- Each guide includes:
  - Installation command
  - Code snippet with syntax highlighting
  - Configuration explanation
  - Verification steps
- Uses mono font (Geist Mono) for code snippets

### 5. Export & Reporting — NOT BUILT (the remaining half of Phase 5)
**Goal:** Generate shareable reports of API contract stability for team communication.

**Status (2026-09-23).** Nothing here exists, and this is now **the whole of what is left of
Phase 5** — §6 shipped its other half. There is no report generator, no scheduler and no
export view. The only export route is `/api/export/typescript`, which emits `.d.ts` interface
definitions from a locked baseline — a contract artifact, not a report. `go.mod` has no
dependencies and the repo contains zero `time.Ticker`, so "scheduled email reports" needs
machinery nobody has written yet. Reports and schedules are **per-project**, which is why this
follows Phase 4 rather than preceding it.

One thing this section no longer has to invent: **the delivery path exists.** A scheduled
report should reuse the deliverer's transport, its SSRF posture and its delivery records
rather than growing a second outbound mechanism beside them — a report that bypassed
`netguard` would be a second, weaker door into the same operator-named address space. What is
genuinely new here is the **generator** (only `contract.GenerateTypeScriptInterfaces` exists,
and it is pure and reusable) and the **scheduler** (a `time.Ticker` owned by `main`, new
machinery — the deliverer deliberately uses `time.NewTimer` so the ticker arrives with
schedules or not at all).

One caution that belongs in the implementation rather than a footnote: **a scheduled report
must not leak.** `SamplePayload` is stored raw by design, so a report that includes it exports
whatever token or email happened to be in a stored response. The AI sidecar already redacts at
its boundary — carry that lesson over rather than rediscovering it in someone's inbox.

Note that §5's colour prescription is legitimate as written: a stability summary is contract
state, so the healthy/warning/breaking roles are the right ones there.

**Features:**
- PDF/PNG export of current dashboard view
- Scheduled email reports (daily/weekly)
- Contract stability summary (percentage healthy, warnings, breaking)
- Trend analysis (improving/degrading/stable)
- Customizable date ranges
- Team sharing via shareable links

**Implementation:**
- Export button in header (next to share button)
- Export options: Current View, Summary Report, Detailed Timeline
- Uses same Vital Teal/Fault Red/Caution Amber color scheme
- Clean, print-friendly CSS for exports

### 6. Webhook & Alert Integrations ✓ SHIPPED (delivery core)
**Goal:** Send drift alerts to external systems for team notification.

**Status (2026-09-23).** Shipped over six PRs (#67–#71 merged, #72 open), and live in
production since the 2026-09-22 deploy. Drift now leaves the browser tab: a contract alert
raises a real outbound POST to Slack, Microsoft Teams, Discord or a generic endpoint,
retries on a schedule, and files a delivery record the dashboard renders.

The previous status for this section read *"NOT BUILT — component kept on purpose as the
Phase 5 seed"*, and the component was unrouted for a stated reason: its Save and Test
buttons toasted success over empty function bodies. That is no longer the case — the view
is routed as **Alert Delivery**, and every control on it now drives a route that exists.
What shipped first, before any of the UI, was the SSRF guard extraction
(`internal/netguard`), because the deliverer and the proxy both need the same rule and the
import cycle made the obvious design impossible. That PR is the one that touched shipped
security code, deliberately reviewable alone.

**Deliberately out of scope** — each a decision, not an omission:

- **Email/SMTP.** A different transport, a form the UI has no shape for, and `net/smtp` is
  frozen upstream. The four shipped channels are all an HTTP POST differing only in body.
- **Alert filtering, custom templates, rate limiting.** The stated follow-up. The three
  dead "Alert Types" checkboxes were not left lying: they were replaced by one true
  sentence saying that every alert a project raises is delivered, and that filtering is
  not configurable yet.
- **The AI explanation in the payload.** The sidecar's prose arrives up to 8s *after* the
  alert by design, so a payload that sometimes carries a paragraph and sometimes does not
  is worse than one that never does. It is also the only alert field whose content the
  product does not author — excluding it, together with excluding response bodies, is what
  makes the payload **structurally incapable** of leaking a stored token or email, rather
  than merely redacted. Doing it properly means a second, follow-up delivery when the
  explanation lands.
- **Widening the SSE `"alert"` frame to warnings.** One line, but it changes a deliberate
  decision (that frame is an *interrupt* channel, deliberately narrower than the alert
  log). It is what would close the remaining asymmetry — a warning can reach Slack before
  the dashboard's alert count refreshes, because `onNewAlert` → `loadAlerts()` only fires
  on a breaking event. Worth its own ticket.
- **Scheduled reports.** The remaining half of Phase 5; see §5.

**Still open in this section:**

- **The Teams envelope is unverified against a real tenant.** Microsoft retired Office 365
  connectors; the shipped default is a Power Automate workflow webhook taking an Adaptive
  Card. A wrong envelope is a 400 at delivery time — which the Test button surfaces in
  seconds — but it is the one shipped-uncertain thing here and should not be described as
  verified until someone sends to a real tenant.
- **`openapi.LoadFromURL`** (`internal/openapi/openapi.go:129`) remains an unnetted
  outbound GET: no timeout, no SSRF check, unbounded `io.ReadAll`. Moving the guards made
  it a two-line fix; it was flagged in #67, #69 and #70 and is still not done.
- **Nothing is scheduled and there is no scheduler.** Zero `time.Ticker` in the repo. The
  deliverer's retry uses `time.NewTimer` precisely so the ticker arrives with schedules or
  not at all.

**After the scoped plan — what is worth improving next,** roughly in order of what an
operator would feel first:

- **Tune the constants.** 2 workers, a 64-deep queue, 3 attempts on a 1s/4s backoff, a 30s
  `Retry-After` cap and 200 records per project are *reasoned, not measured*. They sit in
  one block with the reasoning in comments so the first real deployment can correct them.
- **Rate limiting and alert filtering**, the two halves of "do not spam the channel".
  Filtering is the more valuable one: a project with a chatty endpoint currently delivers
  every warning.
- **A second delivery when the explanation lands** — the follow-up that would let the AI
  prose into a channel without weakening the structural guarantee that kept it out of the
  first payload.
- **Per-project routing** — which channel hears about which project — and a retry for
  deliveries dropped on queue saturation, which today is recorded as `dropped` and nothing
  more. That record is honest; it is not yet useful.
- **Persisting delivery records.** They are in-memory with the alerts they describe, which
  is deliberate (`historiesForPersistLocked` exists to keep runtime telemetry out of the
  persisted document), but it means "did last night's alert get through" is unanswerable
  after a restart.
- **Email as a fifth kind**, if anyone needs it — the one item here that is a genuinely
  different transport rather than a variation on the one that exists.

**Features:**
- Configure webhooks for Slack, Microsoft Teams, Discord, email *(email not shipped — see
  above)*
- Customizable alert templates *(not shipped — the follow-up)*
- Alert filtering (only breaking changes, include warnings, etc.) *(not shipped — the
  follow-up; the dead checkboxes that pretended otherwise are gone)*
- Rate limiting to prevent notification spam *(not shipped — the follow-up)*
- Delivery status tracking and retry logic ✓ *(3 attempts on a 1s/4s backoff; 3xx is
  terminal and never retried, because a redirect is the SSRF bypass the URL check cannot
  see; records are capped at 200 per project, newest-first)*

**Implementation:**
- New `/settings` → "Alerts" section *(shipped as a top-level **Alert Delivery** nav item
  instead — "Alerts" was already taken by the contract alert log, and the sibling labels
  name outcomes rather than mechanisms)*
- Form for webhook URL and secret ✓ *(four independently saveable cards; the secret field
  exists on the generic card only — Slack, Teams and Discord authenticate by a token
  inside the URL they gave the operator and ignore headers they do not recognise, so a
  secret there would be a control whose effect nothing reads)*
- Test button to send sample alert ✓ *(synchronous, and it reports what actually happened —
  `Reached Slack (200 in 142ms)` — including the receiver's own error text; the test is
  deliberately not recorded, since a record answers "did my alert get delivered")*
- Uses mono font for JSON payload examples ✓
- accent-primary for active/inactive toggle switches ✓ — *not* Vital Teal: a toggle's
  state is not a contract state, and Vital Teal means healthy. The delivery-records panel
  is the one place severity hues appear here, and correctly: a failed delivery *is* a
  broken thing. `DESIGN.md`'s ban on decorative severity colour is unchanged.

### 7. Custom Alert Thresholds — NOT BUILT (the last unrouted component)
**Goal:** Allow teams to define what constitutes breaking vs non-breaking changes for their context.

**Status (2026-09-23).** Nothing behind this exists, and §6 shipping did not change that — it
changed the *reason* this is unrouted, which is worth stating because it used to be shared.
The shell's doctrine comment at `web/index.html:1528-1542` now records the split: webhooks is
routed because a backend exists, and thresholds "stays out of this table until that changes,
for its own reason rather than by association."

The shell's settings form was
deleted; `saveThresholdConfig` showed "Custom alert thresholds have been saved."
over an empty function body, and the thresholds it collected were read by
nothing, in either the shell or the Go diff engine. There is no threshold
support anywhere in the Go source. `switchTab` deliberately does not route to
thresholds and `CustomAlertThresholds.tsx` collects severities that no code
reads, so a nav entry would put a screen in front of the user whose buttons lie.

The component itself was **not** deleted — it ships in the bundle, unrouted, and
still toasts "Thresholds Saved / Custom alert thresholds have been saved." over a
no-op. It is **kept on purpose**, as the one screen left waiting on a backend that
does not exist; §6's component waited the same way and stopped waiting when its
backend arrived. The feature list below is the original proposal, not a
description of the product.

**Features:**
- Configure severity levels per change type:
  - Type changes (int → string)
  - Required → optional fields
  - Added/removed fields
  - Status code changes
  - Header changes
- Per-endpoint overrides
- Baseline comparison mode (strict vs lenient)
- Preset configurations: "Strict", "Recommended", "Lenient"

**Implementation:**
- New `/settings` → "Thresholds" section
- Table-based configuration with mono font for JSON paths
- Toggle switches using accent-primary (see the note in §6)
- Reset to defaults button
- Explanation tooltips for each option

### 8. Multi-Tenant View (Agency Mode) — PARTIAL, and not multi-tenant
**Goal:** Enable agencies/freelancers to monitor multiple client APIs from one dashboard.

**Status (2026-09-21).** What exists is **namespaced projects within one trusted
operator**, not multi-tenancy. There is no identity anywhere in the system — no
auth, no user, no session — so nothing can be partitioned *by tenant*;
`isLoopbackRequest` (`internal/server/server.go`) remains the only trust
boundary. Per feature:

- Workspace/client switching — **shipped** (project switcher in the header)
- Per-client baseline isolation — **shipped** (per-project histories, traffic, alerts and caps)
- Per-client target — **shipped** (switching project switches what is proxied)
- Aggregate health dashboard showing all clients — **not built**
- Client-specific alert routing — **not built**
- Client onboarding flow — **not built**
- Role-based access (viewer, admin) — **not possible as written**: with no identity there is nothing for a role to attach to. This is not deferred work; it would need an auth model the product does not have.

**Implementation:** the header selector shipped. The client list sidebar and the
"aggregate health showing % healthy clients" did not — the component that
claimed the latter rendered four hardcoded string literals and was deleted in
`c0ef2dd` ("stop inventing numbers").

## Design Consistency Notes

All proposed improvements should follow the existing Driftwood design system:

- **Colors:** Deep Monitor (#09090B) is the dark ground. Four roles mean **contract state
  and nothing else** — healthy, warning, breaking, info (Vital Teal #00C9A7, Caution Amber
  #FF9F0A, Fault Red #FF3B30, Data Sky #5AC8FA in dark mode; each has a darkened light-mode
  counterpart in `frontend-react/src/tokens.css`, so the hexes above are only half the
  story). **accent-primary** (#4096ff dark / #1a6fd4 light) is the brand and interactive
  colour — primary buttons, the active nav item, focus rings, the logo — and is deliberately
  *not* accent-healthy. This line used to omit it, which is why §1, §6 and §7 all reached for
  a semantic role to style chrome. Colour literals live only in `tokens.css`; a component
  that needs a colour references the token.
- **Typography:** Geist for UI, **Geist Mono** for numbers/code — not JetBrains Mono, which
  this doc claimed and the product has never used. Geist Mono also carries the 400 and 500
  weights the shell sets mono at, so the sans and the mono stay on one typeface lineage.
- **Components:** Flat buttons with 6px radius, tactile feedback, status badges with shape + color coding
- **Layout:** Grid-based, no overlapping elements, asymmetric vitals strip
- **Motion:** Spring physics (stiffness: 100, damping: 20), honor prefers-reduced-motion
- **Anti-Patterns:** No emojis, no Inter font, no pure black, no neon glows, no AI copywriting clichés

## Implementation Approach

These improvements can be implemented incrementally, with each as its own PR following the established pattern. **Read each item's Status block before starting it** — most have shipped in part or in full, and only one (§7) still has no backend at all.

1. **Setup Wizard** - Shipped as a view, not a route. See §1.
2. **Scenario Library** - Shipped as a catalogue. The mock-simulator integration (the "Load Scenario" loader) did not ship and is not a stale-branch fix; it needs a shell↔React bridge that does not exist. See §2.
3. **Contract Evolution Timeline** - Shipped in the History view, except the PNG/SVG export. See §3.
4. **Integration Guides** - Shipped. See §4.
5. **Export & Reporting** - Not built; the remaining half of Phase 5, and it reuses §6's deliverer rather than growing a second outbound path. See §5.
6. **Webhook Integrations** - **Shipped**: SSRF guard extraction, persisted per-project config, a real outbound POST with retry, delivery records, a synchronous test route, a routed Alert Delivery view, and HMAC signing for the generic kind. See §6 for what was deliberately left out.
7. **Custom Alert Thresholds** - Not built; the last item with no backend. Its `CustomAlertThresholds.tsx` is still unrouted for the reason recorded in the shell, and the shell's doctrine comment now distinguishes the two cases. See §7.
8. **Multi-Tenant View** - Project switching + per-project state isolation (partial; see §8)

Each should include:
- Test-first approach (unit/integration tests)
- Manual verification steps
- Documentation in README if user-facing

### Verified debris

Checked against the tree on 2026-09-23, not carried over from an earlier note. Each is real and
none is urgent; they are recorded so the next reader does not have to re-derive them, and so a
claim that *used* to be on this list but is no longer true does not get repeated:

- `ViewHeader`'s `lead` prop (`frontend-react/src/components/ui.tsx:289`) is declared, rendered
  at `:307`, and passed by **no caller anywhere in the tree**. Either a view wants it or it
  should go.
- Three earlier entries were checked and **are no longer true**, so they are struck rather than
  inherited: the "12 root planning artifacts" are gone (the root holds only `README`,
  `CONTRIBUTING`, `DESIGN` and this file); the port-18791 process holding a deleted
  `bin/drift-bin` is gone; and the CSS previously described as dead for the deleted Export view
  (`web/shell.css:1632-1725`) is live Integration Guides styling that §4 ships against.
- `frontend-react/dist/` is a stale local build, but it is **gitignored**
  (`frontend-react/.gitignore:11`), so it is not repository debris and does not belong on a
  cleanup list. `web/dist` is the served one.
- No direct commits to main - branch → PR → user merge workflow