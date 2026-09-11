# Medium-Term Improvements for Driftwood

Following the completion of immediate wins (enhanced empty states, guided tour, improved simulator UX, contextual help, and share functionality), here are proposed medium-term improvements to make Driftwood more powerful and user-friendly.

## Proposed Improvements

### 1. Setup Wizard for First-Time Users ✓ COMPLETED
**Goal:** Guide new users through initial configuration to reduce setup friction.

**Features:**
- Interactive walkthrough for configuring target API
- Auto-detection of common local development servers (localhost:3000, localhost:8080, etc.)
- One-click baseline creation for detected APIs
- Explanation of what Driftwood monitors and how to interpret results
- Option to skip and configure manually

**Implementation:**
- New `/setup` route that shows on first visit if no baselines exist
- Uses same design system (Geist fonts, Vital Teal accents, etc.)
- Progress indicator with 3-4 steps
- "Try with Demo" button that pre-configures the mock simulator

### 2. Scenario Library
**Goal:** Provide pre-configured API breaking change scenarios for learning and experimentation.

**Features:**
- Collection of common API breaking changes (type changes, missing fields, status code changes, etc.)
- Each scenario includes:
  - Description of what changed
  - Expected impact on consumers
  - How to fix it
  - One-click activation
- Filterable by severity (breaking, warning, info) and type (REST, GraphQL, gRPC)
- User-created scenario sharing

**Implementation:**
- New `/scenarios` route in the dashboard
- Scenario cards with Vital Teal/Fault Red/Caution Amber indicators
- "Load Scenario" button that configures mock simulator accordingly
- Integration with guided tour for educational paths

### 3. Contract Evolution Timeline
**Goal:** Visualize how API contracts change over time to help teams understand drift patterns.

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

### 4. Interactive Integration Guides
**Goal:** Provide copy-paste ready integration examples for popular frameworks.

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
- Uses mono font (JetBrains Mono) for code snippets

### 5. Export & Reporting
**Goal:** Generate shareable reports of API contract stability for team communication.

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

### 6. Webhook & Alert Integrations
**Goal:** Send drift alerts to external systems for team notification.

**Features:**
- Configure webhooks for Slack, Microsoft Teams, Discord, email
- Customizable alert templates
- Alert filtering (only breaking changes, include warnings, etc.)
- Rate limiting to prevent notification spam
- Delivery status tracking and retry logic

**Implementation:**
- New `/settings` → "Alerts" section
- Form for webhook URL and secret
- Test button to send sample alert
- Uses mono font for JSON payload examples
- Vital Teal for active/inactive toggle switches

### 7. Custom Alert Thresholds ✓ COMPLETED
**Goal:** Allow teams to define what constitutes breaking vs non-breaking changes for their context.

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
- Toggle switches with Vital Teal accent
- Reset to defaults button
- Explanation tooltips for each option

### 8. Multi-Tenant View (Agency Mode) ✓ COMPLETED
**Goal:** Enable agencies/freelancers to monitor multiple client APIs from one dashboard.

**Features:**
- Workspace/client switching
- Per-client baseline isolation
- Aggregate health dashboard showing all clients
- Client-specific alert routing
- Role-based access (viewer, admin)
- Client onboarding flow

**Implementation:**
- New workspace selector in header (next to driftwood logo)
- Client list sidebar (collapsible)
- Aggregate health showing % healthy clients
- Uses same design system with client-specific labeling
- Mono-spaced client IDs for consistency

## Design Consistency Notes

All proposed improvements should follow the existing Driftwood design system:

- **Colors:** Deep Monitor (#09090B), Vital Teal (#00C9A7), Fault Red (#FF3B30), Caution Amber (#FF9F0A), Data Sky (#5AC8FA)
- **Typography:** Geist for UI, JetBrains Mono for numbers/code
- **Components:** Flat buttons with 6px radius, tactile feedback, status badges with shape + color coding
- **Layout:** Grid-based, no overlapping elements, asymmetric vitals strip
- **Motion:** Spring physics (stiffness: 100, damping: 20), honor prefers-reduced-motion
- **Anti-Patterns:** No emojis, no Inter font, no pure black, no neon glows, no AI copywriting clichés

## Implementation Approach

These improvements can be implemented incrementally, with each as its own PR following the established pattern:

1. **Setup Wizard** - New route + state management
2. **Scenario Library** - New route + mock simulator integration  
3. **Contract Evolution Timeline** - Enhance baseline view + storage history
4. **Integration Guides** - New static content route
5. **Export & Reporting** - Enhance share/export functionality
6. **Webhook Integrations** - New settings section + background worker
7. **Custom Alert Thresholds** - New settings section + diff engine configuration ✓ COMPLETED
8. **Multi-Tenant View** - Workspace routing + state isolation ✓ COMPLETED

Each should include:
- Test-first approach (unit/integration tests)
- Manual verification steps
- Documentation in README if user-facing
- No direct commits to main - branch → PR → user merge workflow