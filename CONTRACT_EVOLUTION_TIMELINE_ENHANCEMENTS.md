# Enhancements to Driftwood Contract Evolution Timeline

Based on the review of the Contract Evolution Timeline implementation, here are the specific enhancements needed to match the specification in MEDIUM_TERM_IMPROVEMENTS.md:

## 1. Enhanced Version Node with Change Type Visualization

**Current Code (lines 2513-2582):**
```javascript
function renderVersionNode(version, index, sortedVersions, lockedVersion) {
  const { Version, CreatedAt, SamplePayload } = version;
  const isLocked = Version === lockedVersion;

  // For this initial implementation, we'll show all versions as neutral
  // In a full implementation, we would compare with previous version to determine change type
  const nodeColor = isLocked ? 'var(--accent-info)' : 'var(--text-muted)';
  const nodeBackground = isLocked ? 'rgba(0, 201, 167, 0.1)' : 'var(--bg-hover)';
  const borderColor = isLocked ? 'var(--accent-info)' : 'var(--border-color)';

  // ... tooltip shows only sample payload
}
```

**Proposed Enhancement:**
```javascript
function renderVersionNode(version, index, sortedVersions, lockedVersion) {
  const { Version, CreatedAt, SamplePayload, changeType, changeDescription, stabilityScore } = version;
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

  // Enhanced tooltip with change information
  const tooltipContent = `
Version ${Version}
${dateStr} ${timeStr}

Change Type: ${changeType.charAt(0).toUpperCase() + changeType.slice(1)}
${changeDescription || 'No detailed change information available'}

${stabilityScore !== undefined ? `Stability Score: ${(stabilityScore * 100).toFixed(1)}%` : ''}
${isLocked ? '(Currently Locked Baseline)' : ''}`;

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
        " title="${tooltipContent.trim()}">
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

## 2. Enhanced Backend Data Structure

The `/ _driftwood/api/histories` endpoint should be enhanced to include:
- `changeType`: 'healthy', 'breaking', or 'warning' indicating the type of change from the previous version
- `changeDescription`: Detailed description of what changed for the tooltip
- `stabilityScore`: Percentage (0-1) of requests matching the baseline for this version/time period

## 3. Add Contract Stability Score Visualization

Add a stability score chart above or below the timeline:

```javascript
// Add this to renderContractEvolutionTimeline function before the timeline
const stabilityChart = `
<div style="margin-bottom: 1.5rem;">
  <h3 style="color: var(--text-main); margin-bottom: 0.5rem;">Contract Stability Score</h3>
  <div style="background: var(--bg-card); border: 1px solid var(--border-color); border-radius: 8px; padding: 1rem;">
    <!-- Simple text-based stability indicator for now -->
    <div style="display: flex; justify-content: space-between; align-items: center;">
      <span>Current Stability:</span>
      <span style="font-weight: 600; font-family: 'JetBrains Mono', monospace;">
        ${stabilityScore !== undefined ? 
          `<span style="color: ${stabilityScore >= 0.9 ? 'var(--accent-healthy)' : stabilityScore >= 0.7 ? 'var(--accent-warning)' : 'var(--accent-breaking)'};">
            ${(stabilityScore * 100).toFixed(1)}%
          </span>` : 'N/A'}
      </span>
    </div>
    <div style="margin-top: 0.5rem; font-size: 0.85rem; color: var(--text-muted);">
      Percentage of requests matching the baseline contract
    </div>
  </div>
</div>
`;

// Then insert it in the container.innerHTML before the timeline
container.innerHTML = `
  <div style="padding: 1.5rem;">
    <h2 style="color: var(--text-main); margin-bottom: 1.5rem; display: flex; align-items: center; gap: 0.5rem;">
      <span style="font-size: 1.5rem;">📈</span>
      Contract Evolution Timeline
    </h2>
    ${stabilityChart}
    <div style="background: var(--bg-card); border: 1px solid var(--border-color); border-radius: 8px;">
      ${histories.map(history => renderEndpointTimeline(history)).join('')}
    </div>
  </div>
`;
```

## 4. Add Version Comparison Functionality

Implement version selection and side-by-side diff view:

```javascript
// Add state tracking for selected versions
let selectedVersions = new Set();

// Modify renderVersionNode to add click handlers:
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
  cursor: pointer;
  position: relative;
  ${selectedVersions.has(Version) ? 'border-color: var(--accent-info); box-shadow: 0 0 0 3px rgba(90, 200, 250, 0.5);' : ''}
" 
onclick="toggleVersionSelection(${Version})"
title="${tooltipContent.trim()}">
  ${nodeSymbol}
  ${isLocked ? '<div style="position: absolute; bottom: -5px; right: -5px; width: 12px; height: 12px; background: var(--accent-info); border-radius: 50%;"></div>' : ''}
</div>

// Add the toggle function:
function toggleVersionSelection(version) {
  if (selectedVersions.has(version)) {
    selectedVersions.delete(version);
  } else {
    if (selectedVersions.size >= 2) {
      // Remove oldest selection if we already have 2
      const oldest = Math.min(...selectedVersions);
      selectedVersions.delete(oldest);
    }
    selectedVersions.add(version);
  }
  
  // Update UI to reflect selection state
  document.querySelectorAll('[onclick^="toggleVersionSelection"]').forEach(el => {
    const versionNum = parseInt(el.getAttribute('onclick').match(/\d+/)[0]);
    if (selectedVersions.has(versionNum)) {
      el.style.borderColor = 'var(--accent-info)';
      el.style.boxShadow = '0 0 0 3px rgba(90, 200, 250, 0.5)';
    } else {
      el.style.borderColor = '';
      el.style.boxShadow = '';
    }
  });
  
  // Show comparison if we have 2 versions selected
  if (selectedVersions.size === 2) {
    showVersionComparison([...selectedVersions]);
  } else {
    hideVersionComparison();
  }
}

// Add comparison view container to the timeline view
// Add comparison controls:
<div style="margin-top: 1rem; padding: 1rem; background: var(--bg-card); border-radius: 8px; border: 1px solid var(--border-color);">
  <h3 style="color: var(--text-main); margin-bottom: 0.5rem;">Version Comparison</h3>
  <div id="version-comparison-container" style="display: none;">
    <!-- Comparison view will be rendered here -->
  </div>
  <div style="margin-top: 0.5rem; text-align: right;">
    <button class="btn btn-sm" onclick="clearVersionSelection()">Clear Selection</button>
  </div>
</div>

// Add the comparison functions:
function showVersionComparison(versions) {
  const container = document.getElementById('version-comparison-container');
  container.style.display = 'block';
  // In a full implementation, this would fetch detailed diff data
  container.innerHTML = `
    <p>Comparing versions ${versions[0]} and ${versions[1]}</p>
    <p style="color: var(--text-muted); font-size: 0.85rem;">
      Detailed diff view would be shown here in a full implementation.
      This would require enhanced backend API to provide version-to-version diff data.
    </p>
  `;
}

function hideVersionComparison() {
  document.getElementById('version-comparison-container').style.display = 'none';
}

function clearVersionSelection() {
  selectedVersions.clear();
  document.querySelectorAll('[onclick^="toggleVersionSelection"]').forEach(el => {
    el.style.borderColor = '';
    el.style.boxShadow = '';
  });
  hideVersionComparison();
}
```

## 5. Add Export Functionality

Add export buttons for PNG/SVG export:

```javascript
// Add export button to the timeline header:
<h2 style="color: var(--text-main); margin-bottom: 1.5rem; display: flex; align-items: center; gap: 0.5rem;">
  <span style="font-size: 1.5rem;">📈</span>
  Contract Evolution Timeline
  <div style="margin-left: auto;">
    <button class="btn btn-sm" onclick="exportTimeline('png')" title="Export as PNG">
      📥 PNG
    </button>
    <button class="btn btn-sm" onclick="exportTimeline('svg')" title="Export as SVG">
      📥 SVG
    </button>
  </div>
</h2>

// Add export functions:
function exportTimeline(format) {
  showToast('Export ${format.toUpperCase()}', `Export functionality for ${format} format is planned for a future update.`);
  // In a full implementation, this would use html2canvas or similar library
  // to convert the timeline view to the requested format
}
```

These enhancements would fully realize the Contract Evolution Timeline feature as specified in MEDIUM_TERM_IMPROVEMENTS.md, providing users with a powerful visualization tool for understanding how their API contracts change over time.