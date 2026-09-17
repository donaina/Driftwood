import React, { useMemo } from 'react';
import { Button, DownloadIcon, LockIcon, Panel, PanelTitle } from './ui';

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
  onExportTimeline: (format: string, endpointKey: string) => void;
  onToggleLock: (history: HistoryItem, version: number, lock: boolean) => void;
  onConfirm: (history: HistoryItem, version: number) => void;
}

const EndpointHistory: React.FC<EndpointHistoryProps> = ({
  history,
  selectedVersionsMap,
  onToggleVersionSelection,
  onClearVersionSelection,
  onExportTimeline,
  onToggleLock,
  onConfirm,
}) => {
  const endpointKey = `${history.method}:${history.path}`;

  // Compute enhanced versions (same logic as in the original code)
  const enhancedVersions = useMemo(() => {
    const sortedVersions = [...history.versions].sort(
      (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    );

    // Nothing here is invented. The backend does not yet report what changed
    // between two versions, so changeType and stabilityScore stay undefined and
    // the node renders an em-dash rather than a number.
    //
    // This previously drew both from Math.random(): a changeType picked from a
    // three-element array and a stability score of 0.5 + random() * 0.5, which
    // the tooltip above then printed to one decimal as a measured percentage
    // and the node colour read as a red/green verdict. Drift detection is only
    // worth anything if its numbers can be trusted, and these changed on every
    // render.
    return sortedVersions.map((version, index) => {
      // The first version has nothing before it to differ from, so its
      // description is a fact rather than a guess. Everything after it is
      // genuinely unknown, and says so.
      const isFirst = index === 0;

      return {
        ...version,
        changeType: undefined,
        changeDescription: isFirst
          ? 'Initial version'
          : 'Change details not captured yet',
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
          <div className="flex justify-between items-center gap-4">
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
        <div className="flex justify-between items-start">
          <div className="flex items-center space-x-3">
            <span className={`method-badge method-${history.method.toLowerCase()}`}>
              {history.method}
            </span>
            <span className="font-mono ml-2 font-semibold">
              {history.path}
            </span>
          </div>
          <div className="text-right space-y-1">
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
      <div className="flex justify-between items-start mb-4">
        <div className="flex items-center space-x-3">
          <span className={`method-badge method-${history.method.toLowerCase()}`}>
            {history.method}
          </span>
          <span className="font-mono ml-2 font-semibold">
            {history.path}
          </span>
        </div>
        <div className="text-right space-y-1">
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
        <div className="flex justify-between items-center mb-4">
          <PanelTitle>
            Contract Evolution Timeline
          </PanelTitle>
          <div className="flex space-x-2">
            <Button size="sm" onClick={() => onExportTimeline('png', endpointKey)}>
              <DownloadIcon />
              PNG
            </Button>
            <Button size="sm" onClick={() => onExportTimeline('svg', endpointKey)}>
              <DownloadIcon />
              SVG
            </Button>
            <Button size="sm" onClick={() => onClearVersionSelection(endpointKey)}
            >
              Clear Selection
            </Button>
          </div>
        </div>
      </Panel>

      <div className="flex items-start space-x-4">
        {enhancedVersions.map((version, index) =>
          renderVersionNode(version, index)
        )}
      </div>

      {selectedVersions.length === 2 && (
        <div className="mt-6">
          <Panel pad="sm">
            <PanelTitle className="mb-2">
              Version Comparison
            </PanelTitle>
            <p className="text-text-muted">
              Comparing versions {selectedVersions[0]} and {selectedVersions[1]}
            </p>
            <p className="mt-2 text-xs text-text-muted">
              Detailed diff view would be shown here in a full implementation.
              This would require enhanced backend API to provide version-to-version diff data.
            </p>
          </Panel>
        </div>
      )}
    </Panel>
  );
};

export default EndpointHistory;