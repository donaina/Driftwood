import React, { useMemo } from 'react';

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
  locked_version?: number;
}

interface EnhancedVersion extends Omit<HistoryItem['versions'][0], 'sample_payload'> {
  changeType?: 'healthy' | 'breaking' | 'warning';
  changeDescription?: string;
  stabilityScore?: number;
}

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

    // Determine appearance values
    const getNodeColor = () => {
      if (isLocked) return 'var(--accent-info)'; // Data Sky for locked version
      switch (changeType) {
        case 'healthy': return 'var(--accent-healthy)'; // Vital Teal
        case 'breaking': return 'var(--accent-breaking)'; // Fault Red
        case 'warning': return 'var(--accent-warning)'; // Caution Amber
        default: return 'var(--text-muted)';
      }
    };

    const getNodeBackground = () => {
      if (isLocked) return 'rgba(90, 200, 250, 0.1)';
      switch (changeType) {
        case 'healthy': return 'rgba(0, 201, 167, 0.1)';
        case 'breaking': return 'rgba(255, 59, 48, 0.1)';
        case 'warning': return 'rgba(255, 159, 10, 0.1)';
        default: return 'var(--bg-hover)';
      }
    };

    const getBorderColor = () => {
      if (isLocked) return 'var(--accent-info)';
      switch (changeType) {
        case 'healthy': return 'var(--accent-healthy)';
        case 'breaking': return 'var(--accent-breaking)';
        case 'warning': return 'var(--accent-warning)';
        default: return 'var(--border-color)';
      }
    };

    const getNodeSymbol = () => {
      if (isLocked) return '🔒'; // Lock symbol for currently locked version
      switch (changeType) {
        case 'healthy': return '●';
        case 'breaking': return '■';
        case 'warning': return '▲';
        default: return '○';
      }
    };

    return (
      <div
        key={versionNumber}
        className="relative cursor-help"
        onClick={() => onToggleVersionSelection(endpointKey, versionNumber)}
        title={tooltipContent.trim()}
      >
        <div className="flex flex-col items-center">
          <div className={`w-10 h-10 flex items-center justify-center rounded-full
            ${isSelected ? 'border-2 border-accent-info' : 'border'}
            ${isSelected ? 'shadow-[0_0_0_3px_rgba(90,200,250,0.5)]' : ''}
            bg-${isLocked ? 'accent-info/10' : getNodeBackground().includes('var(--') ?
                  getNodeBackground().replace('var(--', '').replace(')', '') : getNodeBackground()}
            border-${isSelected ? '2' : '1'} ${isSelected ? 'border-accent-info' : getBorderColor().includes('var(--') ?
                  getBorderColor().replace('var(--', '').replace(')', '') : getBorderColor()}`}>
            <span className={`text-${isLocked ? 'accent-info' : getNodeColor().includes('var(--') ?
                  getNodeColor().replace('var(--', '').replace(')', '') : getNodeColor()} font-bold`}>
              {getNodeSymbol()}
            </span>
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
            className={`mt-2 px-2 py-1 rounded-lg border text-xs transition-colors cursor-pointer ${
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
            {isLocked ? '🔒 Locked' : 'Lock'}
          </button>
          {isProvisional && (
            /* Confirming is offered on the version itself rather than on the
               endpoint, because the question "was this the right shape?" is a
               question about one captured response. Until someone answers it,
               the node reads as unconfirmed and the comparison it drives is
               only as trustworthy as that guess. */
            <button
              type="button"
              className="mt-1 px-2 py-1 rounded-lg border border-accent-warning text-accent-warning text-xs transition-colors cursor-pointer hover:bg-bg-hover"
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
    /* This used to average a Math.random() value per version and print the
       result to one decimal under the caption "Percentage of requests matching
       the baseline contract" — a number none of that arithmetic computed, that
       changed on every render, and that coloured itself green or red as a
       verdict. Claiming a measured contract metric you did not measure is
       worse than showing nothing, so it shows nothing until the backend
       records what actually changed between versions. */
    return (
      <div className="mb-6">
        <h3 className="text-xl font-semibold text-text-main mb-2">
          Contract Stability Score
        </h3>
        <div className="bg-bg-card rounded-xl border border-border-color p-4">
          <div className="flex justify-between items-center">
            <span className="text-sm font-medium text-text-muted">
              Current Stability:
            </span>
            <span className="font-mono text-text-muted font-semibold text-xl">
              —
            </span>
          </div>
          <div className="mt-2 text-xs text-text-muted">
            Not measured yet — Driftwood does not currently record what changed
            between two versions of an endpoint.
          </div>
        </div>
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
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
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
      </div>
    );
  }

  return (
    <div className="bg-bg-card rounded-xl border border-border-color p-6">
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

      <div className="bg-bg-card rounded-xl border border-border-color p-4">
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-xl font-semibold text-text-main">
            Contract Evolution Timeline
          </h3>
          <div className="flex space-x-2">
            <button
              className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
              onClick={() => onExportTimeline('png', endpointKey)}
            >
              📥 PNG
            </button>
            <button
              className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
              onClick={() => onExportTimeline('svg', endpointKey)}
            >
              📥 SVG
            </button>
            <button
              className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
              onClick={() => onClearVersionSelection(endpointKey)}
            >
              Clear Selection
            </button>
          </div>
        </div>
      </div>

      <div className="flex items-start space-x-4">
        {enhancedVersions.map((version, index) =>
          renderVersionNode(version, index)
        )}
      </div>

      {selectedVersions.length === 2 && (
        <div className="mt-6">
          <div className="bg-bg-card rounded-xl border border-border-color p-4">
            <h3 className="text-xl font-semibold text-text-main mb-2">
              Version Comparison
            </h3>
            <p className="text-text-muted">
              Comparing versions {selectedVersions[0]} and {selectedVersions[1]}
            </p>
            <p className="mt-2 text-xs text-text-muted">
              Detailed diff view would be shown here in a full implementation.
              This would require enhanced backend API to provide version-to-version diff data.
            </p>
          </div>
        </div>
      )}
    </div>
  );
};

export default EndpointHistory;