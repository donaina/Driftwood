import React, { useState, useEffect } from 'react';
import { Button, EmptyState, Panel, PanelTitle } from './ui';

/* §2's four roles are contract state and nothing else (DESIGN.md:131). Severity
   is three of them; the union is named once here rather than respelled in the
   interface, the badge map and the badge renderer, which is three chances for
   the third copy to drift from the other two. */
type Severity = 'breaking' | 'warning' | 'info';

/* The API style a scenario exercises. Narrowed from `string` so the filter below
   can be typed against it: the filter matches by string equality, so 'grpc' or
   'graphql' in this data would compile, render, and then silently select
   nothing. These three values are exactly the ones the catalogue uses. */
type Protocol = 'REST' | 'GraphQL' | 'gRPC';

/* Only the two grades anything in this catalogue holds. 'Advanced' is a
   one-word change the day a scenario earns it; declaring it now would be a
   grade nothing carries. */
type Difficulty = 'Beginner' | 'Intermediate';

interface Scenario {
  id: string;
  title: string;
  description: string;
  severity: Severity;
  type: Protocol;
  impact: string;
  fix: string;
  /* Required, and deliberately: every entry here is graded, so the compiler
     refuses a new scenario that isn't. `notes` is optional because it genuinely
     is — only the scenarios the built-in simulator cannot reproduce carry one. */
  difficulty: Difficulty;
  notes?: string;
  changes?: Array<{
    field: string;
    change: string;
    type: Severity;
  }>;
}

const scenarios: Scenario[] = [
  /* --- REST --------------------------------------------------------------
     The first three survive from this component's original hand-written list;
     the rest are ported from the `scenario-library` branch, whose array
     replaced the vanilla renderer's list wholesale and so dropped
     `header-addition` and `field-type-change` on the way. Declaration order is
     the display order, so the protocols are grouped rather than interleaved.
     ---------------------------------------------------------------------- */
  {
    id: 'header-addition',
    title: 'Added Header: X-API-Version',
    description: 'Adding a version header helps clients identify the API version they are consuming.',
    severity: 'info',
    type: 'REST',
    impact: 'Low - Clients can optionally use the version header',
    fix: 'Document the header usage and maintain backward compatibility',
    difficulty: 'Beginner',
    notes: 'Requires a manual API change to test - the Contract Drift Simulator rewrites response bodies, not headers, so there is no simulator mode for this.',
    changes: [
      { field: 'Response header', change: 'X-API-Version → added', type: 'info' }
    ]
  },
  {
    id: 'field-type-change',
    title: 'Changed Field Type: count from integer to string',
    description: 'Changing the count field from integer to string breaks clients expecting numeric values.',
    severity: 'breaking',
    type: 'REST',
    impact: 'High - Clients parsing count as integer will fail',
    fix: 'Use a new field name (e.g., count_str) or keep both fields during transition',
    difficulty: 'Intermediate',
    changes: [
      { field: 'Response body.count', change: 'integer → string', type: 'breaking' }
    ]
  },
  {
    id: 'header-removal',
    title: 'Removed Header: X-RateLimit-Remaining',
    description: 'Removing a custom rate limit header breaks clients that monitor their API usage.',
    severity: 'warning',
    type: 'REST',
    impact: 'Medium - Clients lose visibility into rate limiting',
    fix: 'Provide alternative header or keep deprecated header for transition period',
    difficulty: 'Intermediate',
    notes: 'Requires a manual API change to test - the Contract Drift Simulator rewrites response bodies, not headers, so there is no simulator mode for this.',
    changes: [
      { field: 'Response header', change: 'X-RateLimit-Remaining → removed', type: 'warning' }
    ]
  },
  {
    id: 'type-change-number-to-string',
    title: 'ID Type Change: Number to String',
    description: 'Changing ID fields from numeric to string types breaks clients that expect numeric IDs for mathematical operations or direct database lookups.',
    severity: 'breaking',
    type: 'REST',
    impact: 'High - May cause runtime errors in client applications',
    fix: 'Update client code to handle string IDs, or use content negotiation for versioned endpoints',
    difficulty: 'Intermediate',
    changes: [
      { field: 'user.id', change: 'number → string', type: 'breaking' },
      { field: 'order.customerId', change: 'integer → string', type: 'breaking' }
    ]
  },
  {
    id: 'removed-required-field',
    title: 'Removed Required Field: Email',
    description: 'Removing a required email field from a user registration endpoint breaks clients that depend on this field for account creation or communication.',
    severity: 'breaking',
    type: 'REST',
    impact: 'High - Clients will receive validation errors',
    fix: 'Make the field optional first, then remove in a future version after deprecation period',
    difficulty: 'Intermediate',
    changes: [
      { field: 'user.email', change: 'required → removed', type: 'breaking' }
    ]
  },
  {
    id: 'status-code-change',
    title: 'Status Code Change: 200 → 201',
    description: 'Changing the success status code from 200 to 201 for a creation endpoint may break clients that hardcode status code checks.',
    severity: 'warning',
    type: 'REST',
    impact: 'Medium - May cause incorrect error handling in clients',
    fix: 'Maintain backward compatibility by supporting both status codes during transition',
    difficulty: 'Beginner',
    notes: 'Requires a manual API change to test - the Contract Drift Simulator has no status-code mode, so this demonstrates the concept only.',
    changes: [
      { field: 'POST /users response', change: '200 → 201', type: 'warning' }
    ]
  },
  {
    id: 'added-optional-field',
    title: 'Added Optional Field: Phone Number',
    description: 'Adding an optional phone number field is backward compatible and safe for existing clients.',
    severity: 'info',
    type: 'REST',
    impact: 'Low - Safe addition that enhances functionality',
    fix: 'No action needed - this is a safe change',
    difficulty: 'Beginner',
    changes: [
      { field: 'user.phoneNumber', change: 'added (optional)', type: 'info' }
    ]
  },

  /* --- GraphQL ----------------------------------------------------------- */
  {
    id: 'graphql-type-rename',
    title: 'GraphQL Type Rename: User → Person',
    description: 'Renaming a GraphQL type breaks existing queries that reference the old type name.',
    severity: 'breaking',
    type: 'GraphQL',
    impact: 'High - All existing queries will fail',
    fix: 'Use aliases or maintain both type names during migration period',
    difficulty: 'Intermediate',
    notes: 'Requires manual setup to test - the Contract Drift Simulator serves JSON only, so reproducing this needs a real GraphQL endpoint with the type renamed.',
    changes: [
      { field: 'GraphQL schema', change: 'type User → type Person', type: 'breaking' }
    ]
  },

  /* --- gRPC -------------------------------------------------------------- */
  {
    id: 'grpc-method-rename',
    title: 'gRPC Method Rename: GetUser → FetchUser',
    description: 'Renaming a gRPC method breaks existing client calls that use the old method name.',
    severity: 'breaking',
    type: 'gRPC',
    impact: 'High - All existing gRPC clients will fail with "method not found" errors',
    fix: 'Maintain both method names during transition or use aliases',
    difficulty: 'Intermediate',
    notes: 'Requires manual gRPC work to test - the Contract Drift Simulator has no gRPC mode, so reproducing this means renaming the method in the .proto and regenerating the service.',
    changes: [
      { field: 'Service method', change: 'GetUser → FetchUser', type: 'breaking' }
    ]
  },
  {
    id: 'grpc-required-field-removed',
    title: 'gRPC Removed Required Field: user_id in Request',
    description: "Removing a required field from a gRPC request message breaks clients that don't provide the field.",
    severity: 'breaking',
    type: 'gRPC',
    impact: 'High - Clients will receive parsing errors',
    fix: 'Make the field optional first, then remove after deprecation period',
    difficulty: 'Intermediate',
    notes: 'Requires manual protobuf work to test - the Contract Drift Simulator has no gRPC mode, so reproducing this means editing the .proto and regenerating the service.',
    changes: [
      { field: 'Request.user_id', change: 'required → optional', type: 'warning' },
      { field: 'Request.user_id', change: 'optional → removed', type: 'breaking' }
    ]
  },
  {
    id: 'grpc-enum-value-removed',
    title: 'gRPC Removed Enum Value: Status.UNKNOWN',
    description: 'Removing an enum value breaks clients that send or expect the removed value.',
    severity: 'breaking',
    type: 'gRPC',
    impact: 'Medium - Clients sending the removed value will receive parsing errors',
    fix: 'Deprecate the value first, then remove after sufficient transition period',
    difficulty: 'Beginner',
    notes: 'Requires manual protobuf work to test - the Contract Drift Simulator has no gRPC mode, so reproducing this means removing the enum value and regenerating the service.',
    changes: [
      { field: 'Status enum', change: 'UNKNOWN → removed', type: 'breaking' }
    ]
  }
];

/* The three change severities a scenario carries, and the §4 badge each one
   gets. §2 keeps the colours exclusive — teal healthy, amber warning, red
   breaking, sky info — and §4 carries the same distinction in shape, so the
   badge still reads for someone who cannot tell Fault Red from Vital Teal. */
const SEVERITY: Record<
  Severity,
  { badge: string; shape: string; label: string }
> = {
  breaking: { badge: 'badge-breaking', shape: '■', label: 'BREAKING' },
  warning: { badge: 'badge-warning', shape: '▲', label: 'WARNING' },
  info: { badge: 'badge-info', shape: '●', label: 'INFO' },
};

/* Two filters, two axes, two pieces of state — and that is the point. The ids
   are the same values the scenario data is keyed by ('all' aside), so the rows
   filter on the keys the data actually has. They cannot share one state: both
   axes have an 'all' id, so a single value could only mean one axis at a time,
   and picking a protocol would clear the severity while both 'all' buttons lit
   up together. */
const SEVERITY_FILTERS: Array<{ id: 'all' | Severity; label: string }> = [
  { id: 'all', label: 'All Scenarios' },
  { id: 'breaking', label: 'Breaking' },
  { id: 'warning', label: 'Warnings' },
  { id: 'info', label: 'Informational' },
];

const PROTOCOL_FILTERS: Array<{ id: 'all' | Protocol; label: string }> = [
  { id: 'all', label: 'All Protocols' },
  { id: 'REST', label: 'REST' },
  { id: 'GraphQL', label: 'GraphQL' },
  { id: 'gRPC', label: 'gRPC' },
];

const ScenarioLibrary: React.FC = () => {
  const [activeSeverity, setActiveSeverity] = useState<'all' | Severity>('all');
  const [activeProtocol, setActiveProtocol] = useState<'all' | Protocol>('all');
  const [activeScenario, setActiveScenario] = useState<Scenario | null>(null);

  const filteredScenarios = scenarios.filter(
    (s) =>
      (activeSeverity === 'all' || s.severity === activeSeverity) &&
      (activeProtocol === 'all' || s.type === activeProtocol)
  );

  useEffect(() => {
    const handleShowScenario = () => {
      const root = document.getElementById('scenario-react-root');
      if (root) {
        (root as HTMLElement).style.display = 'block';
      }
    };
    window.addEventListener('showScenario', handleShowScenario);
    return () => {
      window.removeEventListener('showScenario', handleShowScenario);
    };
  }, []);

  /* §4's status badge: pill, 1px tinted border at 30%, fill at 15%. The shell
     already implements exactly that as .contract-badge + .badge-*, so this
     reuses it rather than restating the values — the shell and the React views
     ship one stylesheet, and two definitions of the same badge is how they end
     up disagreeing.

     What stood here was a 32px square with the whole word inside it, and the
     colour came from `text-${textColor}`. Both halves were broken. "BREAKING"
     measures 81px inside a 32px box, so all three badges spilled out of their
     own background. And the interpolated class name only resolved because other
     files happened to contain the same literal — Tailwind's scanner cannot see
     a name assembled at runtime, so the colour that worked was really being
     generated by an unrelated file, and would have disappeared the first time
     that file was tidied. */
  const renderSeverityBadge = (severity: Severity) => {
    const { badge, shape, label } = SEVERITY[severity];
    return (
      <span className={`contract-badge ${badge}`}>
        <span aria-hidden="true">{shape}</span>
        {label}
      </span>
    );
  };

  /* Protocol and difficulty are attributes of a scenario, not contract states,
     so §2 (DESIGN.md:131) leaves them no hue: severity is the only thing in this
     component allowed to spend one. The branch this content came from painted
     Beginner teal, Intermediate amber and Advanced red, which spends three of
     the four reserved roles on a fact about effort — a Beginner scenario
     wearing Caution Amber reads as a warning about the contract. §4's shape
     alphabet (● ▲ ■ ○) is reserved the same way and difficulty needs none of it
     either: dual coding exists to back up a colour, and there is no colour here
     to back up. `.label-caps` is the shell's own small-caps metadata register,
     used for the same kind of thing at web/index.html:1313 and :1323. */
  const renderMeta = (scenario: Scenario) => (
    <p className="text-sm text-text-muted">
      {scenario.type}
      {' · '}
      <span className="label-caps">{scenario.difficulty}</span>
    </p>
  );

  /* The caveat says whether the built-in simulator can produce this at all,
     which is not contract state, so it takes no severity hue either.
     `.delta-item warning` was the other candidate and is wrong twice over: it
     would tint a .badge-warning card amber a second time, and it would spend a
     reserved hue on a testability fact. `.callout` — the shell's neutral
     explanatory block, accent bar on a tinted ground — is the same shape of
     thing as its existing call site, the AI explanation hanging off an alert
     (web/index.html:1297). It renders on the card as well as in the detail
     view: whether a scenario can be reproduced is worth knowing before the
     click, which is the whole point of a catalogue. */
  const renderNote = (notes?: string) =>
    notes ? (
      <div className="callout text-text-muted">
        <strong>Note:</strong> {notes}
      </div>
    ) : null;

  /* The `field` half of a change is the informative one — it says what moved —
     and it was never rendered: every row printed the same literal label
     "Changes:" and then the change text, so `user.id` was dropped and rows were
     indistinguishable. `key={index}` has to stay index-based rather than keyed
     by `field`, because grpc-required-field-removed is a two-step change to the
     same field and would otherwise collide. The per-change `type` stays
     unrendered deliberately: the scenario badge is the authoritative severity,
     and that scenario mixes a warning step into a breaking one on purpose. */
  const renderChanges = (changes: Scenario['changes']) =>
    changes && changes.length > 0 ? (
      <div className="mt-4">
        <PanelTitle level={4} size="base" className="mb-2">Changes</PanelTitle>
        <div className="space-y-2">
          {changes.map((change, index) => (
            <div key={index} className="bg-bg-hover rounded-lg p-3">
              {/* Stacks below `sm` rather than wrapping: side by side at phone
                  width, `justify-between` strands the change value alone on its
                  own line and pushes it to the right edge, away from the field
                  it belongs to. */}
              <div className="flex flex-col gap-1 sm:flex-row sm:flex-wrap sm:justify-between sm:gap-2">
                <span className="font-mono text-sm text-text-muted">{change.field}</span>
                <span className="font-mono text-text-main">{change.change}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    ) : null;

  // Handle active scenario view
  if (activeScenario) {
    return (
      <div className="space-y-8">
        <div className="text-center">
          <h2 className="text-2xl font-semibold tracking-tight text-text-main mb-4">Scenario Library</h2>
          <p className="text-text-muted max-w-xl mx-auto">
            Learn about API contract changes with pre-configured breaking change scenarios
          </p>
        </div>

        <Panel>
          {/* Same unshrinkable row as the cards below — see the comment on the
              scenario grid for why `flex-wrap` and `min-w-0` are both needed. */}
          <div className="flex justify-between items-start gap-y-3 mb-4 max-[479px]:flex-wrap">
            <div className="flex items-center space-x-3 min-w-0">
              {renderSeverityBadge(activeScenario.severity)}
              <div className="min-w-0">
                <PanelTitle className="mb-1 wrap-anywhere">
                  {activeScenario.title}
                </PanelTitle>
                {renderMeta(activeScenario)}
              </div>
            </div>
            <Button size="sm" onClick={() => {
                setActiveScenario(null);
                const scenarioRoot = document.getElementById('scenario-react-root');
                if (scenarioRoot) (scenarioRoot as HTMLElement).style.display = 'none';
                const webhookRoot = document.getElementById('webhook-react-root');
                if (webhookRoot) (webhookRoot as HTMLElement).style.display = 'none';
                document.getElementById('view-traffic')!.classList.add('active');
                document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
                document.getElementById('nav-traffic')!.classList.add('active');
              }}>
              Back to Scenarios
            </Button>
          </div>

          <div className="space-y-4">
            <p className="text-base text-text-muted">{activeScenario.description}</p>

            {renderNote(activeScenario.notes)}

            <div className="grid gap-4">
              <div>
                <PanelTitle level={4} size="base" className="mb-2">Impact</PanelTitle>
                <p className="text-base text-text-muted">{activeScenario.impact}</p>
              </div>
            </div>

            {renderChanges(activeScenario.changes)}

            <div className="mt-4">
              <PanelTitle level={4} size="base" className="mb-2">Fix</PanelTitle>
              <p className="text-base text-text-muted">{activeScenario.fix}</p>
            </div>
          </div>
        </Panel>
      </div>
    );
  }

  // Handle scenario grid view
  return (
    <div className="space-y-8">
      <div className="text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-text-main mb-4">Scenario Library</h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Learn about API contract changes with pre-configured breaking change scenarios
        </p>
      </div>

      {/* Filters — two axes, each labelled. The role/aria-label is load-bearing
          rather than decoration: without it a screen reader reads eight buttons
          in a row, two of them called "All …", with nothing saying which axis
          each belongs to. */}
      <Panel>
        <div className="space-y-4">
          <div role="group" aria-label="Filter by severity">
            <p className="label-caps text-text-muted mb-2">Severity</p>
            <div className="flex flex-wrap gap-2">
              {SEVERITY_FILTERS.map((f) => (
                <Button
                  key={f.id}
                  size="md"
                  variant={activeSeverity === f.id ? 'primary' : 'secondary'}
                  aria-pressed={activeSeverity === f.id}
                  onClick={() => setActiveSeverity(f.id)}
                >
                  {f.label}
                </Button>
              ))}
            </div>
          </div>

          <div role="group" aria-label="Filter by protocol">
            <p className="label-caps text-text-muted mb-2">Protocol</p>
            <div className="flex flex-wrap gap-2">
              {PROTOCOL_FILTERS.map((f) => (
                <Button
                  key={f.id}
                  size="md"
                  variant={activeProtocol === f.id ? 'primary' : 'secondary'}
                  aria-pressed={activeProtocol === f.id}
                  onClick={() => setActiveProtocol(f.id)}
                >
                  {f.label}
                </Button>
              ))}
            </div>
          </div>
        </div>
      </Panel>

      {/* Scenarios Grid */}
      <div className="grid gap-6">
        {/* Reachable for the first time now that there are two axes: every
            GraphQL and gRPC scenario in the catalogue is breaking, so pairing
            either with warning or informational selects nothing. The copy names
            the real coverage rather than apologising, so an empty grid reads as
            the catalogue's shape instead of as a broken view. */}
        {filteredScenarios.length === 0 && (
          <EmptyState
            title="No scenarios match those filters"
            body="The catalogue spans REST, GraphQL and gRPC, but every GraphQL and gRPC scenario is a breaking change. Widen the severity filter, or choose All Protocols, to see the rest."
            action={
              <Button
                variant="primary"
                onClick={() => {
                  setActiveSeverity('all');
                  setActiveProtocol('all');
                }}
              >
                Show All Scenarios
              </Button>
            }
          />
        )}

        {filteredScenarios.map(scenario => (
          /* `min-w-0` has to sit on the grid item, not only inside it. A
             single-column grid sizes its track to the widest item's min-content
             and refuses to go below it, and that floor comes from the track's
             automatic minimum — which only `min-width: 0` on the item itself
             removes. Shrinking the row's children alone does nothing: the badge
             (an inline-flex pill), the title (wraps only at its longest word)
             and the nowrap button still summed to 451px of min-content inside a
             352px panel at 400px, so every card was that width and the panel
             scrolled sideways to hide it — invisibly, because overflow-y:auto
             makes overflow-x compute to auto too. */
          <Panel key={scenario.id} className="min-w-0">
            {/* `flex-wrap` is scoped to `max-[479px]`, the shell's own narrowest
                tier, rather than left on unconditionally. Unscoped it is decided
                by the row's *content* width, so cards with long titles would drop
                their button while short-titled ones in the same column kept it
                inline — a ragged, per-card difference across the whole 480–746px
                band. Below 479px every card wraps, so the tier reads uniformly;
                at 480px and above nothing wraps and the row is unchanged from
                before this fix, which still fits because `min-w-0` lets the title
                take the shortfall. */}
            <div className="flex justify-between items-start gap-y-3 mb-4 max-[479px]:flex-wrap">
              <div className="flex items-center space-x-3 min-w-0">
                {renderSeverityBadge(scenario.severity)}
                <div className="min-w-0">
                  {/* `wrap-anywhere` (overflow-wrap:anywhere) and not
                      `break-words`: only `anywhere` also lowers the element's
                      intrinsic min-content, which is what lets the card shrink
                      past its longest word. `break-word` looks identical and
                      leaves the floor in place. It breaks mid-word only when a
                      single word genuinely has no room — at 320px,
                      "Status.UNKNOWN" — and never at 360px or above. */}
                  <PanelTitle className="mb-1 wrap-anywhere">
                    {scenario.title}
                  </PanelTitle>
                  {renderMeta(scenario)}
                </div>
              </div>

              <Button size="sm" onClick={() => setActiveScenario(scenario)}>
                View Details
              </Button>
            </div>

            <div className="space-y-4">
              <p className="text-base text-text-muted">{scenario.description}</p>

              {renderNote(scenario.notes)}

              <div className="grid gap-4">
                <div>
                  <PanelTitle level={4} size="base" className="mb-2">Impact</PanelTitle>
                  <p className="text-base text-text-muted">{scenario.impact}</p>
                </div>
              </div>

              {renderChanges(scenario.changes)}
            </div>
          </Panel>
        ))}
      </div>
    </div>
  );
};

export default ScenarioLibrary;
