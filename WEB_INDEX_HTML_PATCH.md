# Proposed Enhancements to Contract Evolution Timeline in web/index.html

Based on the review of the Contract Evolution Timeline implementation, here are the specific enhancements needed to match the specification in MEDIUM_TERM_IMPROVEMENTS.md:

## 1. Enhance Version Node Coloring (lines 2513-2582)

**Current Code:**
```javascript
function renderVersionNode(version, index, sortedVersions, lockedVersion) {
  const { Version, CreatedAt, SamplePayload } = version;
  const isLocked = Version === lockedVersion;

  // For this initial implementation, we'll show all versions as neutral
  // In a full implementation, we would compare with previous version to determine change type
  const nodeColor = isLocked ? 'var(--accent-info)' : 'var(--text-muted)';
  const nodeBackground = isLocked ? 'rgba(0, 201, 167, 0.1)' : 'var(--bg-hover)';
  const borderColor = isLocked ? 'var(--accent-info)' : 'var(--border-color)';

  // ... rest of function
}
```

**Proposed Enhancement:**
```javascript
function renderVersionNode(version, index, sortedVersions, lockedVersion) {
  const { Version, CreatedAt, SamplePayload, changeType } = version;
  const isLocked = Version === lockedVersion;

  // Determine node appearance based on change type
  let nodeColor, nodeBackground, borderColor, nodeSymbol;
  
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
    }
  }

  // Format timestamp
  const date = new Date(CreatedAt);
  const timeStr = date.toLocaleTimeString();
  const dateStr = date.toLocaleDateString();

  // Create a sample of the payload for the tooltip
  const payloadSample = JSON.stringify(SamplePayload, null, 2);
  const truncatedSample = payloadSample.length > 100
    ? payloadSample.substring(0, 100) + '...'
    : payloadSample;

  // Get change description for tooltip
  const changeDescription = version.changeDescription || 
    (changeType === 'healthy' ? 'No significant changes detected' :
     changeType === 'breaking' ? 'Breaking changes detected - immediate action required' :
     changeType === 'warning' ? 'Warning changes detected - review recommended' :
     'Version information');

  return `
    <div style="text-align: center; min-width: 80px; position: relative;">
      <div style="
        position: relative;
        display: inline-flex;
        flex-direction: column;
        align-items: center;
      ">
        <!-- Version Node -->
        <div style="
          width: 40px;
          height: 40px;
          border: 2px solid ${borderColor};
          border-radius: 50%;
          background: ${nodeBackground};
          color: ${nodeColor};
          display: flex;
          align-items: center;
          justify-content: center;
          font-weight: 600;
          font-size: 0.9rem;
          cursor: help;
          position: relative;
        " title="Version ${Version}
${dateStr} ${timeStr}

Change Type: ${changeType.charAt(0).toUpperCase() + changeType.slice(1)}
${changeDescription}">
          ${nodeSymbol}
          ${isLocked ? '<div style="position: absolute; bottom: -5px; right: -5px; width: 12px; height: 12px; background: var(--accent-info); border-radius: 50%;"></div>' : ''}
        </div>

        <!-- Connector line to next version (except for last version) -->
        ${index < sortedVersions.length - 1 ? `
          <div style="
            width: 2px;
            height: 20px;
            background: var(--border-color);
            margin: 8px auto 0;
          "></div>
        ` : ''}
      </div>

      <div style="margin-top: 0.5rem; font-size: 0.75rem; color: var(--text-muted);">
        v${Version}
      </div>
    </div>
  `;
}
```

## 2. Enhance Backend Data Structure

The `/ _driftwood/api/histories` endpoint should be enhanced to include:
- `changeType`: 'healthy', 'breaking', or 'warning' indicating the type of change from the previous version
- `changeDescription`: Detailed description of what changed for the tooltip

## 3. Add Contract Stability Score Visualization

Add a stability score chart above or below the timeline showing percentage of requests matching baseline over time.

## 4. Add Version Comparison Functionality

Implement version selection and side-by-side diff view.

## 5. Add Export Functionality

Add export buttons for PNG/SVG export of the timeline view.