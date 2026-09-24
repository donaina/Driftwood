import React, { useMemo } from 'react';
import { Button, LockIcon, Panel, PanelTitle } from './ui';
import VersionComparison from './VersionComparison';

/* One node of an inferred or declared schema, tagged the way pkg/types/types.go
   serialises JSONSchemaNode. It is recursive, and `sample_value` is deliberately
   `unknown`: the store strips per-field sample values before persisting, so
   anything this side reads out of it would be a value the API does not send. */
export interface JSONSchemaNode {
  type: string;
  nullable?: boolean;
  properties?: Record<string, JSONSchemaNode>;
  item_schema?: JSONSchemaNode;
  sample_value?: unknown;
  required_keys?: string[];
  format?: string;
}

/* The wire type, spelled the way `/_driftwood/api/histories` actually
   serialises it. pkg/types/types.go tags EndpointHistory and ContractBaseline
   lower_snake, and server.go encodes those structs straight to the response.

   These fields were PascalCase. That is not a style disagreement: the API has
   never emitted `Method`, `Versions` or `CreatedAt`, so every one of these
   reads was undefined, `[...history.Versions]` threw "not iterable" inside
   render, React unmounted the tree, and the Version History view came up
   empty with nothing in the UI to explain why. */
export interface HistoryItem {
  method: string;
  path: string;
  versions: Array<{
    version: number;
    created_at: string;
    sample_payload: string;
    /* The shape this version captured. It is on every version the API sends —
       TestHistoriesCarryEachVersionsSchema pins that — and it is what
       `/_driftwood/api/histories/diff` compares, which is why the comparison
       panel can exist at all. Declared rather than ignored because a wire type
       that describes only the fields one screen happens to read is how the
       PascalCase bug above survived: nothing in this file could tell a field
       that was missing from a field that was never there. */
    schema?: JSONSchemaNode;
    /* Where this version came from: 'auto' when Driftwood captured it from live
       traffic, 'manual' when a human accepted or confirmed it, 'openapi' when it
       came from an imported spec. Absent on baselines written before the field
       existed, which read as provisional — see types.BaselineSource*. */
    source?: string;
  }>;
  observation_count: number;
  /* The bounded window of recent sightings, oldest first — the same series
     ObservationCount totals. In-memory only: storage strips it on persist, so
     a fresh process legitimately has none and the stability panel says so
     rather than showing a stale trend from the last run.

     This was missing from the wire type even though the API has always sent
     it, which is why the stability panel had nothing to compute from. */
  observations?: Array<{
    timestamp: string;
    status_code: number;
    duration_ms: number;
    /* NO_BASELINE, MATCH, WARNING or BREAKING — see types.go. */
    contract_status: string;
  }>;
  locked_version?: number;
}

interface EnhancedVersion extends Omit<HistoryItem['versions'][0], 'sample_payload'> {
  changeType?: 'healthy' | 'breaking' | 'warning';
  changeDescription?: string;
  stabilityScore?: number;
}

/* §4 asks status to be carried by shape and colour together, so the timeline
   still reads for someone who cannot separate Fault Red from Vital Teal. Shape
   is the half that survives greyscale, colourblindness, and a screenshot
   pasted into a ticket. */
const SEVERITY_SHAPE: Record<'healthy' | 'breaking' | 'warning' | 'unknown', string> = {
  healthy: '●',
  breaking: '■',
  warning: '▲',
  unknown: '○',
};

/* §1's contract-stability sparkline: contract match rate across the endpoint's
   recent observations, oldest on the left. Flat and high means the contract has
   held for the whole window; a cliff is the request where it stopped holding,
   which is the one thing a single "current stability" percentage cannot show,
   because it has no memory of when things changed.

   The line is the RUNNING match rate, not a per-request pass/fail. A per-request
   series over a healthy endpoint is a solid block of one value and reads as a
   filled rectangle; the running rate starts wherever the first observation put
   it and settles as evidence accumulates, so the shape carries the trend.

   Drawn with preserveAspectRatio="none" and vector-effect="non-scaling-stroke"
   so the geometry fills its 120px slot while the stroke stays an honest 1.5px —
   the same construction as the traffic table's row sparkline. */
const StabilitySparkline: React.FC<{
  observations: NonNullable<HistoryItem['observations']>;
  className?: string;
}> = ({ observations, className = '' }) => {
  const W = 120;
  const H = 24;
  const PAD = 2;

  // Only sightings that had a contract to be measured against; a NO_BASELINE
  // request neither held nor broke anything.
  const rated = observations.filter((o) => o.contract_status !== 'NO_BASELINE');
  if (rated.length === 0) return null;

  let matched = 0;
  const series = rated.map((o, i) => {
    if (o.contract_status === 'MATCH') matched += 1;
    return matched / (i + 1);
  });

  const stepX = series.length > 1 ? (W - PAD * 2) / (series.length - 1) : 0;
  const points = series
    .map((v, i) => {
      const x = PAD + i * stepX;
      const y = H - PAD - v * (H - PAD * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');

  return (
    <svg
      className={`w-[120px] h-[24px] shrink-0 ${className}`}
      viewBox={`0 0 ${W} ${H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={`Contract match rate over the last ${rated.length} observed requests`}
    >
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
};

interface EndpointHistoryProps {
  history: HistoryItem;
  selectedVersionsMap: Map<string, number[]>;
  onToggleVersionSelection: (endpointKey: string, version: number) => void;
  onClearVersionSelection: (endpointKey: string) => void;
  onToggleLock: (history: HistoryItem, version: number, lock: boolean) => void;
  onConfirm: (history: HistoryItem, version: number) => void;
}

/* A path is a run of segments a browser will not break between: it breaks after
   a hyphen, but not after a slash, so mostly what it has is one long word —
   which is why the header row below could not shrink and overflowed the card.
   `wrap-anywhere` fixes the overflow, but on its own it lets the line-breaking
   fill each line to its last fitting character, so at 320px the middle of a
   word was being cut in half: `/api/v2/organizat` / `ions/7/members/42` / …
   That reads as corruption, not as a wrapped path.

   A `<wbr>` after each slash supplies the break points a path actually has, and
   the browser prefers a real break opportunity over the overflow-wrap fallback.
   Measured on `/api/v2/organizations/7/members/42/notification-preferences`:
   at 320px this wraps in five lines all breaking at slashes, where without it
   the same path wrapped in four with `organizat` / `ions` cut in half. It costs
   one line at 390px — four rather than three — because a line that ends at a
   slash cannot also begin with one, so each line here starts with its own
   segment. At 480px and above the two are identical.

   `wrap-anywhere` stays on the element around this, as the guarantee rather
   than the preference: if a single segment is ever wider than the card — a
   long slug, an ID — that is what keeps it inside rather than overflowing. */
const PathText: React.FC<{ path: string }> = ({ path }) => (
  <>
    {path.split('/').map((segment, index) => (
      <React.Fragment key={index}>
        {index > 0 && '/'}
        {segment}
        <wbr />
      </React.Fragment>
    ))}
  </>
);

const EndpointHistory: React.FC<EndpointHistoryProps> = ({
  history,
  selectedVersionsMap,
  onToggleVersionSelection,
  onClearVersionSelection,
  onToggleLock,
  onConfirm,
}) => {
  const endpointKey = `${history.method}:${history.path}`;

  // Compute enhanced versions (same logic as in the original code)
  const enhancedVersions = useMemo(() => {
    const sortedVersions = [...history.versions].sort(
      (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    );

    // Nothing here is invented. A node has no per-version change type, because
    // the diff is between two versions somebody picks rather than something
    // computed for every node on the way past — so changeType and stabilityScore
    // stay undefined and the node renders its state symbol rather than a number.
    //
    // This previously drew both from Math.random(): a changeType picked from a
    // three-element array and a stability score of 0.5 + random() * 0.5, which
    // the tooltip above then printed to one decimal as a measured percentage
    // and the node colour read as a red/green verdict. Drift detection is only
    // worth anything if its numbers can be trusted, and these changed on every
    // render.
    return sortedVersions.map((version, index) => {
      // The first version has nothing before it to differ from, so its
      // description is a fact rather than a guess. Everything after it says
      // where the answer is, which used to be "Change details not captured yet"
      // — true when the only diff engine lived in the proxy and the dashboard
      // had no route to it, and a lie once the comparison panel below started
      // reading the same one.
      const isFirst = index === 0;

      return {
        ...version,
        changeType: undefined,
        changeDescription: isFirst
          ? 'Initial version'
          : 'Select two versions to compare them',
        stabilityScore: undefined,
      };
    });
  }, [history.versions]);

  const selectedVersions = selectedVersionsMap.get(endpointKey) || [];

  const renderVersionNode = (version: EnhancedVersion, index: number) => {
    const { version: versionNumber, created_at, changeType, changeDescription, stabilityScore, source } = version;

    /* A version captured from live traffic is a guess about the contract: it is
       what the API returned, not what it promised. While it stays unconfirmed,
       a field that was already missing when Driftwood first looked is part of
       the baseline, so drift that predates Driftwood compares as MATCH and is
       invisible — nothing in the process can tell that apart from a healthy API.
       Saying so on the node is the whole point: the badge is not decoration, it
       is the only signal that this contract has not been vouched for. An absent
       source counts as provisional, matching ContractBaseline.IsProvisional. */
    const isProvisional = source === undefined || source === '' || source === 'auto';
    const isLocked = history.locked_version === versionNumber;

    // Format timestamp
    const date = new Date(created_at);
    const timeStr = date.toLocaleTimeString();
    const dateStr = date.toLocaleDateString();

    // Enhanced tooltip with change information
    const tooltipContent = `
      Version ${versionNumber}
      ${dateStr} ${timeStr}

      Change Type: ${changeType ? changeType.charAt(0).toUpperCase() + changeType.slice(1) : 'Unknown'}
      ${changeDescription || 'No detailed change information available'}

      ${stabilityScore !== undefined ? `Stability Score: ${(stabilityScore * 100).toFixed(1)}%` : ''}
      ${isLocked ? '(Currently Locked Baseline)' : ''}
      ${isProvisional ? '(Unconfirmed: captured from live traffic, not yet accepted as the contract)' : ''}`;

    // Check if this version is selected for comparison
    const isSelected = selectedVersions.includes(versionNumber);

    /* The node has a handful of named states, so its appearance is a class name
       rather than a thing assembled at render time.

       It used to be assembled: the old code took `var(--border-color)`, chopped
       off the "var(" and the ")", put a prefix back on, and interpolated the
       result into a template literal. Tailwind builds its utilities by scanning
       source for complete class strings, so none of what that produced was ever
       generated, and the pieces that *did* resolve only did so because some
       unrelated file happened to contain the same words. See the .version-node
       rules in shell.css for what the user actually saw.

       Locked outranks severity: pinning is a decision somebody made, severity
       is something Driftwood observed. */
    const nodeState = isLocked
      ? 'version-node--locked'
      : changeType
        ? `version-node--${changeType}`
        : '';

    // A drawn padlock for the locked state, §4's shapes for the rest. An emoji
    // could not stand in: it ignores currentColor, so it would not take the
    // colour that carries the other half of the meaning.
    const nodeSymbol = isLocked ? <LockIcon /> : SEVERITY_SHAPE[changeType ?? 'unknown'];

    return (
      <div
        key={versionNumber}
        className="relative cursor-help"
        onClick={() => onToggleVersionSelection(endpointKey, versionNumber)}
        title={tooltipContent.trim()}
      >
        <div className="flex flex-col items-center">
          <div className={`version-node ${nodeState} ${isSelected ? 'is-selected' : ''}`}>
            {/* Decorative: the state is already in the title above and in the
                lock button below, and a screen reader announcing "black
                circle" adds nothing to either. */}
            <span aria-hidden="true">{nodeSymbol}</span>
          </div>
          {index < enhancedVersions.length - 1 && (
            <div className="w-px h-4 mt-2 bg-border-color"></div>
          )}
          <div className="mt-2 text-xs text-text-muted">
            v{versionNumber}
          </div>
          {/* Locking is a separate act from accepting a version. Accepting a new
              shape records it; pinning decides which accepted shape the endpoint
              is still held to — so that promoting v4 does not quietly become the
              thing every later response is compared against. The button is a
              button rather than a click handler on the node because the node
              already means "select for comparison", and one target with two
              meanings is how the badge ended up decorative. */}
          <button
            type="button"
            className={`mt-2 px-2 py-1 rounded-sm border text-xs transition-all duration-100 active:translate-y-px cursor-pointer ${
              isLocked
                ? 'border-accent-info text-accent-info'
                : 'border-border-color text-text-muted hover:bg-bg-hover hover:text-text-main'
            }`}
            aria-pressed={isLocked}
            title={
              isLocked
                ? `Release v${versionNumber}: this endpoint will track its latest version again`
                : `Hold this endpoint to v${versionNumber} as its contract`
            }
            onClick={(event) => {
              event.stopPropagation();
              onToggleLock(history, versionNumber, !isLocked);
            }}
          >
            {isLocked ? <><LockIcon /> Locked</> : 'Lock'}
          </button>
          {isProvisional && (
            /* Confirming is offered on the version itself rather than on the
               endpoint, because the question "was this the right shape?" is a
               question about one captured response. Until someone answers it,
               the node reads as unconfirmed and the comparison it drives is
               only as trustworthy as that guess. */
            <button
              type="button"
              className="mt-1 px-2 py-1 rounded-sm border border-accent-warning text-accent-warning text-xs transition-all duration-100 active:translate-y-px cursor-pointer hover:bg-bg-hover"
              title={`Accept v${versionNumber} as the contract for this endpoint. Until you do, drift is measured against a response Driftwood merely observed, so a problem that was already there will not be reported.`}
              onClick={(event) => {
                event.stopPropagation();
                onConfirm(history, versionNumber);
              }}
            >
              Unconfirmed — confirm
            </button>
          )}
        </div>
      </div>
    );
  };

  const renderStabilityChart = () => {
    /* §1's signature element, and the reason the observation window exists.

       What this must not be is what it was: the old version averaged a
       Math.random() value per version and printed it to one decimal under the
       caption "Percentage of requests matching the baseline contract" — a
       number none of that arithmetic computed, that changed on every render,
       and that coloured itself green or red as a verdict. It was replaced with
       an em-dash, which was honest but useless.

       It is now a real measurement over the real series. `observations` is the
       endpoint's own recent sightings, each carrying the ContractStatus the
       proxy computed for it at the time. Match rate over that window IS the
       question the caption asks, so it no longer has to be approximated.

       Two details keep the number honest:

       - Requests with no contract to compare against are excluded from the
         denominator rather than counted as failures. An endpoint that has been
         observed but never locked cannot fail to match a baseline it does not
         have, and folding NO_BASELINE in would report a brand-new endpoint as
         0% stable.
       - The panel says how many requests the figure covers. A rate over three
         sightings and a rate over fifty are not the same claim. */
    const observations = history.observations ?? [];
    const checked = observations.filter((o) => o.contract_status !== 'NO_BASELINE');
    const matched = checked.filter((o) => o.contract_status === 'MATCH').length;

    if (checked.length === 0) {
      return (
        <div className="mb-6">
          <PanelTitle className="mb-2">Contract Stability</PanelTitle>
          <Panel pad="sm">
            <div className="flex justify-between items-center">
              <span className="text-sm font-medium text-text-muted">
                Current stability:
              </span>
              <span className="font-mono text-text-muted font-semibold text-xl">
                —
              </span>
            </div>
            <div className="mt-2 text-xs text-text-muted">
              {observations.length === 0
                ? 'No requests observed yet in this session. Driftwood measures stability from live traffic, so this fills in as soon as your app calls through the proxy.'
                : `Seen ${observations.length} ${observations.length === 1 ? 'request' : 'requests'}, but none had a contract to compare against yet. Lock a version below to start measuring.`}
            </div>
          </Panel>
        </div>
      );
    }

    const rate = matched / checked.length;
    // Same thresholds as the vitals ring, so "stable" means one thing across
    // the dashboard rather than two.
    const tone =
      rate === 1 ? 'text-accent-healthy' : rate >= 0.9 ? 'text-accent-warning' : 'text-accent-breaking';

    return (
      <div className="mb-6">
        <PanelTitle className="mb-2">Contract Stability</PanelTitle>
        <Panel pad="sm">
          {/* Stacked below the shell's narrowest tier, not wrapped: the
              sparkline is a fixed 120px, so the row's min-content is that plus
              the percentage and the label — about 346px — which the card cannot
              hold until the viewport reaches roughly 442px. `flex-wrap` would
              have put the percentage on a line of its own and left it
              left-aligned under the sparkline, where the number reads as
              detached from what it measures. */}
          <div className="flex justify-between items-center gap-4 max-[479px]:flex-col max-[479px]:items-start">
            <div>
              <div className="text-sm font-medium text-text-muted">
                Current stability:
              </div>
              <div className="mt-1 text-xs text-text-muted">
                {matched} of the last {checked.length}{' '}
                {checked.length === 1 ? 'request' : 'requests'} matched the contract.
              </div>
            </div>
            <StabilitySparkline observations={observations} className={tone} />
            <span className={`font-mono font-semibold text-xl ${tone}`}>
              {Math.round(rate * 100)}%
            </span>
          </div>
        </Panel>
      </div>
    );
  };

  /* An endpoint Driftwood has seen traffic for but holds no contract on. This
     is a state worth showing rather than a card to leave blank: it is the
     difference between "nothing is happening on this API" and "this API is
     being served right now and I have never been told what its contract is",
     and it is the state every endpoint starts in. */
  if (history.versions.length === 0) {
    return (
      <Panel>
        {/* Same header treatment as the populated branch below; see the
            comment there for why `min-w-0` and `wrap-anywhere` are both
            needed and why the wrap is scoped to the shell's narrowest tier. */}
        <div className="flex justify-between items-start gap-y-3 max-[479px]:flex-wrap">
          <div className="flex items-center space-x-3 min-w-0">
            <span className={`method-badge method-${history.method.toLowerCase()}`}>
              {history.method}
            </span>
            <span className="font-mono ml-2 font-semibold wrap-anywhere">
              <PathText path={history.path} />
            </span>
          </div>
          <div className="text-right space-y-1 max-[479px]:w-full">
            <div className="text-sm text-text-muted">
              Observations: {history.observation_count}
            </div>
          </div>
        </div>
        <p className="mt-4 text-text-muted">
          No contract accepted for this endpoint yet. Driftwood is recording
          traffic here, but it has nothing to compare it against, so it cannot
          tell you whether this endpoint has drifted.
        </p>
      </Panel>
    );
  }

  return (
    <Panel>
      {/* The path is the row's whole problem. Chrome does not break a line
          after `/` — a hyphen is a break opportunity, a slash is not — so the
          `font-mono` span below behaves as one nearly unbreakable word and its
          full width became the row's min-content: 518px for
          `/api/v2/organizations/7/members/42/notification-preferences` inside a
          290px card at 390px, with `justify-between` pushing the counts out to
          the row's right edge, 260px past the panel. Neither `min-w-0` nor
          `wrap-anywhere` is optional here: `min-w-0` removes the flex item's
          automatic minimum so the group may shrink at all, and `wrap-anywhere`
          (overflow-wrap:anywhere) is the only one of the two overflow-wrap
          values that also lowers the intrinsic min-content — `break-words`
          looks identical and leaves the floor exactly where it was. Measured
          with `overflow-wrap: normal` instead, the row went straight back to
          518px inside a 222px card. */}
      <div className="flex justify-between items-start gap-y-3 mb-4 max-[479px]:flex-wrap">
        <div className="flex items-center space-x-3 min-w-0">
          <span className={`method-badge method-${history.method.toLowerCase()}`}>
            {history.method}
          </span>
          <span className="font-mono ml-2 font-semibold wrap-anywhere">
            <PathText path={history.path} />
          </span>
        </div>
        {/* `w-full` only in the wrapped tier, so the counts keep their place
            at the row's right edge instead of drifting left on their own
            line. */}
        <div className="text-right space-y-1 max-[479px]:w-full">
          <div className="text-sm text-text-muted">
            Versions: {enhancedVersions.length}
          </div>
          <div className="text-sm text-text-muted">
            Observations: {history.observation_count}
          </div>
        </div>
      </div>

      {renderStabilityChart()}

      <Panel pad="sm">
        {/* The three buttons measure 318px and never shrank, so the row's
            min-content was that plus the title — more than the inner panel
            holds below roughly 500px, where `justify-between` then pushed the
            buttons past its right edge. `gap-2` rather than `space-x-2` on the
            group: both space a single line identically, but only `gap` leaves
            the first button of a wrapped line flush instead of indenting it. */}
        <div className="flex justify-between items-center gap-y-2 mb-4 max-[479px]:flex-wrap">
          <PanelTitle className="min-w-0">
            Contract Evolution Timeline
          </PanelTitle>
          {/* PNG and SVG sat here and drew a download icon each, and pressing
              either one raised a toast saying export was planned for a future
              update. Two buttons that announce their own uselessness are the
              same lie as the paragraph this view used to carry about the
              backend, in a smaller box. Export is deferred, so the buttons are
              gone until it exists — the timeline is the thing on screen, and a
              screenshot of it is what a person does today. */}
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => onClearVersionSelection(endpointKey)}
            >
              Clear Selection
            </Button>
          </div>
        </div>
      </Panel>

      {/* Five version nodes are wider than the card below 430px. Each node is a
          circle, a label and a Lock button, and the unconfirmed one adds an
          "Unconfirmed — confirm" button that nothing can shrink, so the first
          node alone takes 88px. Measured, the row overflowed itself by 109px
          inside a 222px card at 320px and 39px at 390px, first fitting at
          430px — and because the overflow was sideways inside the row, every
          node's own box still landed on screen, which is why only a
          `scrollWidth - clientWidth` probe on the row found it.

          `flex-wrap` is the wrong tool here even though it is the right one
          for the rows above. It re-lays items at their max-content size, so a
          node would take about 146px and five of them would become one per
          line at 320px; and because max-content stays wider than the card well
          past the width where the row actually breaks, it would also reflow
          the 430–767px range that fits today.

          So the nodes stack instead, across the shell's narrowest tier. That
          tier is 50px wider than the overflow it fixes — 430–479px previously
          fit and now stack to 680px tall — and the tier is kept anyway because
          the stability row above the timeline already stacks at exactly 479px,
          and two rows in one card breaking at two different widths is the
          raggedness this scope exists to avoid. Measured: the row is 680px
          tall at 320px, and `items-stretch` puts all five circles on one
          vertical axis (left edge 49 at both 320px and 390px) where
          `items-start` would leave each node at its own width and drop every
          circle at a different centre.

          Replacing `space-x-4` with `gap-4` is what makes the column gap
          real — `space-x-*` sets a horizontal margin, which does nothing
          between stacked rows — and above 430px the two are equivalent to the
          pixel, verified by rebuilding the old row and re-measuring: 168px
          tall with nodes at 139,45,45,45,45 at 480px, 152px and
          147,45,45,45,45 at 620px and up, identical to before. */}
      <div className="flex items-start gap-4 max-[479px]:flex-col max-[479px]:items-stretch">
        {enhancedVersions.map((version, index) =>
          renderVersionNode(version, index)
        )}
      </div>

      {/* One click into a two-click action used to say nothing at all. The node
          carries `cursor-help` and a tooltip about the version, and the only
          thing that reveals a node is a selection target is clicking one and
          finding out — which is how a working feature goes on reading as a
          broken one. */}
      {selectedVersions.length === 1 && (
        <p className="mt-6 text-sm text-text-muted">
          v{selectedVersions[0]} selected — select a second version to compare
          them.
        </p>
      )}

      {selectedVersions.length === 2 && (
        <div className="mt-6">
          <VersionComparison history={history} selected={selectedVersions} />
        </div>
      )}
    </Panel>
  );
};

export default EndpointHistory;