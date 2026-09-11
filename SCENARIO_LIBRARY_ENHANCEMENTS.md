# Enhancements to Driftwood Scenario Library

Based on the review and current implementation, here are the recommended enhancements for the Scenario Library feature:

## 1. Enhanced Scenarios Array

Add gRPC scenarios and improve existing scenarios with better structure:

```javascript
const scenarios = [
  // REST SCENARIOS
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
    ],
    mockMode: 'TYPE_BREAK'
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
    ],
    mockMode: 'MISSING_FIELD'
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
    changes: [
      { field: 'POST /users response', change: '200 → 201', type: 'warning' }
    ],
    notes: 'Requires manual API modification to test - demonstrates concept only'
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
    ],
    mockMode: 'NORMAL'
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
    changes: [
      { field: 'Response header', change: 'X-RateLimit-Remaining → removed', type: 'warning' }
    ],
    notes: 'Requires manual API modification to test - demonstrates concept only'
  },
  
  // GRAPHQL SCENARIOS
  {
    id: 'graphql-type-rename',
    title: 'GraphQL Type Rename: User → Person',
    description: 'Renaming a GraphQL type breaks existing queries that reference the old type name.',
    severity: 'breaking',
    type: 'GraphQL',
    impact: 'High - All existing queries will fail',
    fix: 'Use aliases or maintain both type names during migration period',
    difficulty: 'Intermediate',
    changes: [
      { field: 'GraphQL schema', change: 'type User → type Person', type: 'breaking' }
    ],
    notes: 'GraphQL scenarios require manual setup or GraphQL endpoint'
  },
  
  // NEW GRPC SCENARIOS
  {
    id: 'grpc-method-rename',
    title: 'gRPC Method Rename: GetUser → FetchUser',
    description: 'Renaming a gRPC method breaks existing client calls that use the old method name.',
    severity: 'breaking',
    type: 'gRPC',
    impact: 'High - All existing gRPC clients will fail with "method not found" errors',
    fix: 'Maintain both method names during transition or use aliases',
    difficulty: 'Intermediate',
    changes: [
      { field: 'Service method', change: 'GetUser → FetchUser', type: 'breaking' }
    ],
    notes: 'Requires manual gRPC service modification to test'
  },
  {
    id: 'grpc-required-field-removed',
    title: 'gRPC Removed Required Field: user_id in Request',
    description: 'Removing a required field from a gRPC request message breaks clients that don\\'t provide the field.',
    severity: 'breaking',
    type: 'gRPC',
    impact: 'High - Clients will receive parsing errors',
    fix: 'Make the field optional first, then remove after deprecation period',
    difficulty: 'Intermediate',
    changes: [
      { field: 'Request.user_id', change: 'required → optional', type: 'warning' },
      { field: 'Request.user_id', change: 'optional → removed', type: 'breaking' }
    ],
    notes: 'Requires manual protobuf modification to test'
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
    changes: [
      { field: 'Status enum', change: 'UNKNOWN → removed', type: 'breaking' }
    ],
    notes: 'Requires manual protobuf modification to test'
  }
];
```

## 2. Enhanced loadScenario Function

Improve the loadScenario function to provide better user guidance:

```javascript
function loadScenario(scenarioId) {
  const scenario = scenarios.find(s => s.id === scenarioId);
  if (!scenario) return;

  // Hide all pages and show scenario library
  document.querySelectorAll('.page').forEach(page => {
    page.style.display = 'none';
  });
  scenarioLibrarySection.style.display = 'block';

  // Update active filter button
  filterButtons.forEach(btn => {
    btn.classList.toggle('active', btn.getAttribute('data-filter') === scenario.severity || btn.getAttribute('data-filter') === 'all');
  });

  // Configure mock simulator based on scenario
  let mockMode = null;
  let message = `Loading scenario: ${scenario.title}`;
  let isManualSetup = false;

  switch (scenario.id) {
    case 'type-change-number-to-string':
      mockMode = 'TYPE_BREAK';
      message += '\n\n✅ Automatically configured simulator to return breaking type changes (number → string)';
      break;
    case 'removed-required-field':
      mockMode = 'MISSING_FIELD';
      message += '\n\n✅ Automatically configured simulator to remove required fields';
      break;
    case 'added-optional-field':
      mockMode = 'NORMAL';
      message += '\n\n✅ Configured simulator to return normal responses (added optional field is safe)';
      break;
    case 'status-code-change':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for this scenario:\n1. Change your API to return 201 instead of 200 for successful creation\n2. The scenario demonstrates a warning-level change that may affect client error handling\n\n💡 Tip: Use the "Contract Drift Simulator" section above to manually test this change';
      break;
    case 'graphql-type-rename':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for GraphQL scenarios:\n1. Configure a GraphQL endpoint with the modified schema\n2. Rename the type from User to Person\n3. Existing queries referencing "User" will now fail\n\n💡 Tip: This demonstrates a high-impact breaking change in GraphQL APIs';
      break;
    case 'header-removal':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for this scenario:\n1. Remove the X-RateLimit-Remaining header from your API responses\n2. Clients monitoring rate limits will lose visibility\n\n💡 Tip: Demonstrates how removing seemingly innocuous headers can break monitoring clients';
      break;
    case 'grpc-method-rename':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for gRPC scenarios:\n1. Modify your gRPC service to rename the method\n2. Update protobuf and regenerate gRPC code\n3. Existing clients calling the old method will fail\n\n💡 Tip: gRPC method renaming is a breaking change requiring client updates';
      break;
    case 'grpc-required-field-removed':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for this gRPC scenario:\n1. Modify your protobuf to make user_id optional, then remove it\n2. Update your service implementation accordingly\n3. Clients not providing user_id will receive parsing errors\n\n💡 Tip: Demonstrates the two-phase approach for removing required fields';
      break;
    case 'grpc-enum-value-removed':
      isManualSetup = true;
      message += '\n\n⚠️ Manual setup required for this gRPC scenario:\n1. Remove the enum value from your protobuf definition\n2. Update any code that references the removed value\n3. Clients sending the removed value will receive parsing errors\n\n💡 Tip: Enum value removal requires careful consideration in gRPC services';
      break;
    default:
      mockMode = 'NORMAL';
  }

  // Apply mock mode if we have one
  if (mockMode) {
    setMockMode(mockMode);
  }

  // Show appropriate confirmation
  if (isManualSetup) {
    showToast('Scenario Selected - Manual Setup Required', message);
  } else {
    showToast('Scenario Loaded', message);
  }
}
```

## 3. Enhanced Scenario Card Design

Improve the scenario card rendering to show more information:

```javascript
filteredScenarios.forEach(scenario => {
  const card = document.createElement('div');
  card.className = 'scenario-card';
  
  // Determine difficulty color
  let difficultyColor = 'var(--text-muted)';
  switch (scenario.difficulty) {
    case 'Beginner': difficultyColor = 'var(--accent-healthy)'; break;
    case 'Intermediate': difficultyColor = 'var(--accent-warning)'; break;
    case 'Advanced': difficultyColor = 'var(--accent-breaking)'; break;
  }
  
  card.innerHTML = `
    <div class="scenario-card-header">
      <div class="scenario-card-title">
        <div class="scenario-card-badge ${scenario.severity}">${scenario.severity.toUpperCase()}</div>
        <div>
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <div>${scenario.title}</div>
            <div style="font-size: 0.75rem; color: ${difficultyColor};">
              ${scenario.difficulty}
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="scenario-card-body">
      <p class="scenario-card-description">${scenario.description}</p>
      
      ${scenario.notes ? `
      <div style="background: var(--bg-hover); border-radius: 4px; padding: 0.75rem; margin: 1rem 0; font-size: 0.85rem;">
        <strong>Note:</strong> ${scenario.notes}
      </div>
      ` : ''}
      
      <div class="scenario-card-details">
        <div class="scenario-detail-item">
          <span class="scenario-detail-label">Impact:</span>
          <span class="scenario-detail-value">${scenario.impact}</span>
        </div>
        <div class="scenario-detail-item">
          <span class="scenario-detail-label">Type:</span>
          <span class="scenario-detail-value">${scenario.type}</span>
        </div>
        <div class="scenario-detail-item">
          <span class="scenario-detail-label">Fix:</span>
          <span class="scenario-detail-value">${scenario.fix}</span>
        </div>
      </div>
      
      ${scenario.changes && scenario.changes.length > 0 ? `
      <div class="scenario-card-details" style="margin: 1rem 0;">
        <div class="scenario-detail-label">Changes:</div>
        ${scenario.changes.map(change => `
          <div class="scenario-detail-item" style="display: flex; justify-content: space-between;">
            <span class="scenario-detail-label">${change.field}:</span>
            <span class="scenario-detail-value" style="font-family: 'JetBrains Mono', monospace; background: var(--bg-hover); padding: 0.25rem 0.5rem; border-radius: 3px; font-size: 0.85rem;">
              ${change.change}
            </span>
            <span style="color: ${change.type === 'breaking' ? 'var(--accent-breaking)' : change.type === 'warning' ? 'var(--accent-warning)' : 'var(--accent-healthy)'}; font-size: 0.75rem;">
              ${change.type.toUpperCase()}
            </span>
          </div>
        `).join('')}
      </div>
      ` : ''}
    </div>
    <div class="scenario-card-footer">
      <button class="scenario-activate-btn" data-scenario-id="${scenario.id}">
        ${scenario.mockMode ? 'Load Scenario' : 'View Details'}
      </button>
      ${!scenario.mockMode ? '<span style="font-size: 0.75rem; color: var(--text-muted); margin-left: 0.5rem;">(manual setup)</span>' : ''}
    </div>
  `;
  
  scenarioGrid.appendChild(card);
});
```

## 4. Add User-Created Scenario Functionality

Add a "Create Scenario" button and basic functionality:

```javascript
// Add to the scenario library header section
<div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: center;">
  <h2>Scenario Library</h2>
  <button class="btn btn-primary" onclick="showCreateScenarioDialog()">
    + Create Scenario
  </button>
</div>

// Add the dialog function
function showCreateScenarioDialog() {
  // In a full implementation, this would show a modal dialog
  // For now, we'll show a toast indicating the feature is planned
  showToast('Feature Coming Soon', 'User-created scenario sharing will be available in a future update. This will allow you to create, save, and share your own API breaking change scenarios.');
}
```

## 5. Enhanced Filtering

Add gRPC to the filter options:

```html
<div class="scenario-filters">
  <button class="scenario-filter-btn active" data-filter="all">All Scenarios</button>
  <button class="scenario-filter-btn" data-filter="breaking">Breaking</button>
  <button class="scenario-filter-btn" data-filter="warning">Warnings</button>
  <button class="scenario-filter-btn" data-filter="info">Informational</button>
  <button class="scenario-filter-btn" data-filter="REST">REST</button>
  <button class="scenario-filter-btn" data-filter="GraphQL">GraphQL</button>
  <button class="scenario-filter-btn" data-filter="gRPC">gRPC</button>
</div>
```

And update the filtering logic:
```javascript
function renderScenarios(filter = 'all') {
  scenarioGrid.innerHTML = '';

  const filteredScenarios = filter === 'all'
    ? scenarios
    : scenarios.filter(s => s.type === filter || s.severity === filter);

  // ... rest of function
}
```

These enhancements would significantly improve the Scenario Library by:
1. Adding gRPC support as specified in the requirements
2. Improving user guidance for scenarios requiring manual setup
3. Enhancing visual design and information density
4. Laying groundwork for user-created scenario sharing
5. Making the library more useful as a learning and experimentation tool