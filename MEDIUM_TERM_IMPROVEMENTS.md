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
- Selecting two versions renders a **Version Comparison** panel — **shipped
  2026-09-24.** Until then the panel said the feature "would require enhanced backend API",
  which had stopped being true the moment the proxy grew `diff.CompareSchemas`: every version
  in `/api/histories` already carried its schema. It now reads
  `GET /_driftwood/api/histories/diff` and lists the deltas, from that same engine rather than
  a second one written for the dashboard — two implementations would be two answers to the one
  question this product exists to answer.
- Contract-stability sparkline across observed requests — shipped
- **PNG/SVG export — NOT shipped, and off the screen as of 2026-09-24.** The two buttons and
  `handleExportTimeline` are gone. The toast did not claim a file was written, which is what
  separated this from the deleted thresholds form — but the buttons themselves promised an
  artifact that does not exist, and a control whose only effect is a message about the future
  is the same claim as the paragraph the Version Comparison panel used to carry.

  Still on screen as of 2026-09-25: `HistoryView.tsx:165` is the placeholder, and
  `EndpointHistory.tsx:515-521` is the pair of buttons. The answer chosen is the second: the
  buttons come off, because export is §5 and §5 is not built. That is on **#78
  (`fix/real-version-comparison`), open and unmerged at the time of writing** — the same branch
  that replaces the Version Comparison panel's *"This would require enhanced backend API"* excuse
  with a real diff, which is the other soft spot in this view.

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

**Status (2026-09-21; corrected 2026-09-25).** Shipped, as `IntegrationsLibrary.tsx` mounted by
the `nav-integrations` entry — all five frameworks (Express, FastAPI, NestJS, Django, Rails), each
with a code snippet and numbered setup steps.

**"Shipped" was the wrong verdict, and this block is why.** The heading says SHIPPED and the
component does render, so it looked earned. What it renders is not true: every card's
first setup step installs a package that does not exist — `npm install -g @donaina/driftwood`
(`IntegrationsLibrary.tsx:44`, `:86`), `pip install driftwood-proxy` (`:66`, `:103`),
`gem install driftwood-proxy` (`:119`) — and every code block is framework boilerplate with no
Driftwood code in it. A second, older copy with five *different* invented names
(`@donaina/driftwood-proxy`, `driftwood-django`, `driftwood-rails`) is still in `web/index.html`
at `:2271-2365`; it is unreachable, because its renderer bails at `:2322` on
`if (!integrationsGrid) return;` and nothing in the document has that id.

Driftwood is a reverse proxy, so the honest guide is *point your client's base URL at it* — true
for every framework, needing no per-language package. A branch rewriting the component around that
exists as **#77 (`fix/true-integration-guides`), open and unmerged at the time of writing**; until
it lands, this section describes shipped code that names packages the registry does not have.

Deviations from the proposal:

- **"Try in Sandbox" — not built**, and correctly so: there is no sandbox to run anything in,
  so the button would have been a control reporting an action it did not take.
- **No copy button.** "Ready-to-copy" means the snippet is a selectable `<pre>`, not one
  click. Worth adding, and it is real work rather than the fake it would have been before.
- **No framework logos** — the card carries the framework's first letter in a tinted square.
- **One live palette breach**, to fold into the next UI pass: that square is
  `bg-accent-info/20` + `text-accent-info` (`IntegrationsLibrary.tsx:183-184`), which spends
  a contract-state role on chrome — the same mistake §1, §6 and §7 made in prose. It wants
  `accent-primary`. `EndpointHistory.tsx:301` does it too, on the pin button's pressed state.
  (This reference was `:269`; the line moved when the timeline row was reworked.)

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
- **`openapi.LoadFromURL` no longer belongs on this list.** This bullet used to say it was the
  last unnetted outbound GET — no timeout, no SSRF check, unbounded `io.ReadAll` — and that was
  true when it was written. It was closed in `0d09ae5` ("net the OpenAPI spec fetch, and give it
  a deadline"), which routes the URL through `netguard.ParseAndValidate` and dials it through
  `netguard.DialContext` with a 5s timeout, caps the body at `maxSpecBytes` (32 MB) and the
  redirect chain at `maxSpecRedirects` (5, each hop required to stay on the origin host). The
  line reference had also drifted: `LoadFromURL` is now at `internal/openapi/openapi.go:216`, and
  the file around it is `LoadSpec`/`LooksLikeURL`, added when the import route needed the same
  loader as the command line. Recorded rather than deleted, because a reader who remembers the
  complaint should be able to see it answered.
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

### 7. Custom Alert Thresholds — SHIPPED 2026-09-25
**Goal:** Allow teams to define what constitutes breaking vs non-breaking changes for their context.

**Status (2026-09-25).** Shipped, and the goal as worded did not survive contact with the engine.
A project now has an **alert floor per delta kind**: the least severe difference of that kind that
raises an alert. `GET`/`POST /_driftwood/api/thresholds` read and write it, it is persisted per
project in `baselines.json` (`storeVersion` 4), and — the part that makes it real —
`store.AddTraffic` reads it to decide whether an alert exists at all. Before this, that decision was
the literal `HasBreakingChanges || HasWarnings` and the screen configured five severities that
nothing read.

**What the original proposal got wrong, and why:**

- **"Define what constitutes breaking vs non-breaking"** is not something a config can do. Severity
  is a measurement: `BREAKING` means a promise the baseline made was broken. A setting able to
  relabel a delta would let the dashboard show `MATCH` beside a `BREAKING` one, and would make the
  README's severity table describe something other than what the engine does. A floor decides what
  *reaches* you, never what a change *is*.
- **Five rows became seven.** The engine emits seven delta kinds and the old five did not partition
  them: "Added/Removed Fields" configured the same kind as "Required → Optional Fields" above it,
  and **"Header changes" configured no kind at all** — headers are recorded on traffic and never
  diffed, and none of the seven kinds is a header. That row is gone rather than backed, because
  backing it means writing a header comparison and an eighth kind, which is a feature and not a
  backend for a row.
- **The three preset names survive; their meanings moved.** "Strict" used to mean "treat every
  change as breaking", which asks a config to overwrite a measurement. It now means the lowest
  floor (alert on everything), and "Lenient" the highest (alert only on breaks). The preset is
  *derived* from the floors rather than stored beside them, so a name cannot disagree with what it
  describes.

**Features:**
- Configure severity levels per change type — **shipped**, one row per engine kind:
  - Type changes (int → string)
  - Removed fields
  - Added fields
  - Nullability changes
  - Array item type changes
  - Format changes
  - Status code changes
- Per-endpoint overrides — **not built.** The floor is per project. A per-endpoint floor is
  defensible but it is a second dimension on a config with no UI for it, and the store has no
  per-endpoint config of any kind today.
- Baseline comparison mode (strict vs lenient) — **shipped** as the presets.
- Preset configurations: "Strict", "Recommended", "Lenient" — **shipped**, and served with their
  floors so the dashboard never expands a preset name itself.

**Implementation:**
- `GET`/`POST /_driftwood/api/thresholds`, not `/settings` — the settings route is the install-level
  proxy config, and a floor is one project's.
- Table-based configuration, one row per kind, in the engine's own kind order.
- Preset cards, plus a "Custom" card that is **not** a button: custom is what the floors amount to
  when they are not one preset applied uniformly, which is a reading and not a choice.
- Save reports the real outcome, on a per-panel error line rather than by replacing the view.

**What the floor deliberately does not reach.** The proxy's live `alert` frame — the one that raises
a toast — still fires on `HasBreakingChanges` alone. A floor lowered to `INFO` widens the alert log
and what webhooks deliver; it does not make informational changes interrupt anybody. The two were
already separate conditions and this keeps them that way, with the README saying so.

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

These improvements can be implemented incrementally, with each as its own PR following the established pattern. **Read each item's Status block before starting it** — most have shipped in part or in full, and the three that have not (§2's mock-simulator loader, §3's export buttons, §5) are each waiting on machinery that does not exist rather than on wiring.

1. **Setup Wizard** - Shipped as a view, not a route. See §1.
2. **Scenario Library** - Shipped as a catalogue. The mock-simulator integration (the "Load Scenario" loader) did not ship and is not a stale-branch fix; it needs a shell↔React bridge that does not exist. See §2.
3. **Contract Evolution Timeline** - Shipped in the History view, except the PNG/SVG export. See §3.
4. **Integration Guides** - Shipped. See §4.
5. **Export & Reporting** - Not built; the remaining half of Phase 5, and it reuses §6's deliverer rather than growing a second outbound path. See §5.
6. **Webhook Integrations** - **Shipped**: SSRF guard extraction, persisted per-project config, a real outbound POST with retry, delivery records, a synchronous test route, a routed Alert Delivery view, and HMAC signing for the generic kind. See §6 for what was deliberately left out.
7. **Custom Alert Thresholds** - **Shipped**: a per-kind alert floor persisted per project, `GET`/`POST /_driftwood/api/thresholds`, read by `store.AddTraffic` to decide whether an alert exists, and a routed Alert Thresholds view. The five rows became seven — one per delta kind — because "Header changes" configured no kind the engine emits. See §7.
8. **Multi-Tenant View** - Project switching + per-project state isolation (partial; see §8)

Each should include:
- Test-first approach (unit/integration tests)
- Manual verification steps
- Documentation in README if user-facing

### Verified debris

Checked against the tree on 2026-09-25, not carried over from an earlier note. Each is real and
none is urgent; they are recorded so the next reader does not have to re-derive them, and so a
claim that *used* to be on this list but is no longer true does not get repeated:

- **`.integration-*` (`web/shell.css:1608-1731`) is dead CSS, and the earlier note that called it
  live was wrong.** These rules — `.integration-card`, `-code-box`, `-detail-*`, `-verification`,
  `.integrations-filter-btn` — are referenced from exactly one place: the unreachable renderer in
  `web/index.html:2316-2360`. The shipped `IntegrationsLibrary.tsx` is Tailwind and uses none of
  them, so nothing on screen takes these rules. `.integrations-filter-btn.active` (`:1731`) also
  spends `--accent-info` on chrome, which `DESIGN.md` reserves. Both go with §4's rewrite; see #77.
- **`.share-btn` (`web/shell.css:1434-1453`, `:1791`) and `.empty-state-enhanced`
  (`:1458-1473`) have zero references tree-wide** — not in the shell, not in any React component.
  The non-enhanced `.empty-state` (`:682-710`) *is* live and is a different rule.
- Three earlier entries were checked and **are no longer true**, so they are struck rather than
  inherited. `ViewHeader`'s `lead` prop (`frontend-react/src/components/ui.tsx:289`, rendered at
  `:307`) used to have no caller anywhere; §7's view now passes one
  (`CustomAlertThresholds.tsx:272`). The "12 root planning artifacts" are gone — the root holds
  only `README`, `CONTRIBUTING`, `DESIGN` and this file. And the port-18791 process holding a
  deleted `bin/drift-bin` is gone.
- `frontend-react/dist/` is a stale local build, but it is **gitignored**
  (`frontend-react/.gitignore:10`), so it is not repository debris and does not belong on a
  cleanup list. `web/dist` is the served one.
- No direct commits to main - branch → PR → user merge workflow