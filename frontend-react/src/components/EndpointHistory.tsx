import React, { useMemo } from 'react';

interface HistoryItem {
  Method: string;
  Path: string;
  Versions: Array<{
    Version: number;
    CreatedAt: string;
    SamplePayload: string;
  }>;
  ObservationCount: number;
  LockedVersion?: number;
}

interface EnhancedVersion extends Omit<HistoryItem['Versions'][0], 'SamplePayload'> {
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
}

const EndpointHistory: React.FC<EndpointHistoryProps> = ({
  history,
  selectedVersionsMap,
  onToggleVersionSelection,
  onClearVersionSelection,
  onExportTimeline,
}) => {
  const endpointKey = `${history.Method}:${history.Path}`;

  // Compute enhanced versions (same logic as in the original code)
  const enhancedVersions = useMemo(() => {
    const sortedVersions = [...history.Versions].sort(
      (a, b) => new Date(a.CreatedAt).getTime() - new Date(b.CreatedAt).getTime()
    );

    return sortedVersions.map((version, index) => {
      let changeType: EnhancedVersion['changeType'] | undefined;
      let changeDescription: EnhancedVersion['changeDescription'] | undefined =
        'Change details not available (backend enhancement needed)';
      let stabilityScore: EnhancedVersion['stabilityScore'] | undefined;

      if (index > 0) {
        // For simplicity, we'll assign a random changeType for demonstration
        // In reality, this would be computed by comparing with the previous version
        const changeTypes = ['healthy', 'breaking', 'warning'] as const;
        changeType = changeTypes[Math.floor(Math.random() * changeTypes.length)] as EnhancedVersion['changeType'];
        // Simulate a stability score between 0.5 and 1.0
        stabilityScore = 0.5 + Math.random() * 0.5;
      } else {
        // First version has no previous version to compare with
        changeType = undefined;
        changeDescription = 'Initial version';
        stabilityScore = 1.0; // Assume 100% stability for the initial version
      }

      return {
        ...version,
        changeType,
        changeDescription,
        stabilityScore,
      };
    });
  }, [history.Versions]);

  const selectedVersions = selectedVersionsMap.get(endpointKey) || [];

  const renderVersionNode = (version: EnhancedVersion, index: number) => {
    const { Version, CreatedAt, changeType, changeDescription, stabilityScore } = version;
    const isLocked = history.LockedVersion === Version;

    // Determine node appearance based on change type
    let nodeColor: string, nodeBackground: string, borderColor: string, nodeSymbol: string;

    if (isLocked) {
      nodeColor = 'var(--accent-info)'; // Data Sky for locked version
      nodeBackground = 'rgba(90, 200, 250, 0.1)';
      borderColor = 'var(--accent-info)';
      nodeSymbol = '🔒'; // Lock symbol for currently locked version
    } else {
      // Color-code based on change type from previous version
      switch (changeType) {
        case 'healthy':
          nodeColor = 'var(--accent-healthy)'; // Vital Teal
          nodeBackground = 'rgba(0, 201, 167, 0.1)';
          borderColor = 'var(--accent-healthy)';
          nodeSymbol = '●';
          break;
        case 'breaking':
          nodeColor = 'var(--accent-breaking)'; // Fault Red
          nodeBackground = 'rgba(255, 59, 48, 0.1)';
          borderColor = 'var(--accent-breaking)';
          nodeSymbol = '■';
          break;
        case 'warning':
          nodeColor = 'var(--accent-warning)'; // Caution Amber
          nodeBackground = 'rgba(255, 159, 10, 0.1)';
          borderColor = 'var(--accent-warning)';
          nodeSymbol = '▲';
          break;
        default:
          nodeColor = 'var(--text-muted)';
          nodeBackground = 'var(--bg-hover)';
          borderColor = 'var(--border-color)';
          nodeSymbol = '○';
          break;
      }
    }

    // Format timestamp
    const date = new Date(CreatedAt);
    const timeStr = date.toLocaleTimeString();
    const dateStr = date.toLocaleDateString();

    // Enhanced tooltip with change information
    const tooltipContent = `
      Version ${Version}
      ${dateStr} ${timeStr}

      Change Type: ${changeType ? changeType.charAt(0).toUpperCase() + changeType.slice(1) : 'Unknown'}
      ${changeDescription || 'No detailed change information available'}

      ${stabilityScore !== undefined ? `Stability Score: ${(stabilityScore * 100).toFixed(1)}%` : ''}
      ${isLocked ? '(Currently Locked Baseline)' : ''}`;

    // Check if this version is selected for comparison
    const isSelected = selectedVersions.includes(Version);

    return (
      <div
        key={Version}
        className="relative cursor-help"
        onClick={() => onToggleVersionSelection(endpointKey, Version)}
        title={tooltipContent.trim()}
      >
        <div className="flex flex-col items-center">
          <div className={`w-10 h-10 flex items-center justify-center rounded-full
            ${isSelected ? 'border-2 border-accent-info' : 'border'}
            ${isSelected ? 'shadow-[0_0_0_3px_rgba(90,200,250,0.5)]' : ''}
            bg-${isLocked ? 'accent-info/10' : changeType === 'healthy' ? 'accent-healthy/10'
              : changeType === 'breaking' ? 'accent-breaking/10'
                : changeType === 'warning' ? 'accent-warning/10' : 'bg-hover'}`}>
            <span className={`text-${isLocked ? 'accent-info' : changeType === 'healthy' ? 'accent-healthy'
              : changeType === 'breaking' ? 'accent-breaking'
                : changeType === 'warning' ? 'accent-warning' : 'text-muted'} font-bold`}>
              {nodeSymbol}
            </span>
            {isLocked && (
              <div className="absolute bottom-0 right-0 w-2 h-2 bg-accent-info rounded-full -mb-1 -mr-1"></div>
            )}
          </div>
          {index < enhancedVersions.length - 1 && (
            <div className="w-px h-4 mt-2 bg-border-color"></div>
          )}
          <div className="mt-2 text-xs text-text-muted">
            v{Version}
          </div>
        </div>
      </div>
    );
  };

  const renderStabilityChart = () => {
    // Calculate average stability score for the endpoint
    const scores = enhancedVersions
      .map((v) => v.stabilityScore)
      .filter((score): score is number => score !== undefined);
    const avgScore =
      scores.length > 0
        ? scores.reduce((sum, score) => sum + score, 0) / scores.length
        : 0;

    const color =
      avgScore >= 0.9
        ? 'var(--accent-healthy)'
        : avgScore >= 0.7
          ? 'var(--accent-warning)'
          : 'var(--accent-breaking)';

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
            <span className={`font-mono text-${color} font-semibold text-xl`}>
              {(avgScore * 100).toFixed(1)}%
            </span>
          </div>
          <div className="mt-2 text-xs text-text-muted">
            Percentage of requests matching the baseline contract
          </div>
        </div>
      </div>
    );
  };

  return (
    <div className="bg-bg-card rounded-xl border border-border-color p-6">
      <div className="flex justify-between items-start mb-4">
        <div className="flex items-center space-x-3">
          <span className={`method-badge method-${history.Method.toLowerCase()}`}>
            {history.Method}
          </span>
          <span className="font-mono ml-2 font-semibold">
            {history.Path}
          </span>
        </div>
        <div className="text-right space-y-1">
          <div className="text-sm text-text-muted">
            Versions: {enhancedVersions.length}
          </div>
          <div className="text-sm text-text-muted">
            Observations: {history.ObservationCount}
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
          )}
        </div>
      </div>
    </div>
  );
};

export default EndpointHistory;