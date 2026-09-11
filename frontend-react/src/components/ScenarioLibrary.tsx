import React, { useState, useEffect } from 'react';

interface Scenario {
  id: string;
  title: string;
  description: string;
  severity: 'breaking' | 'warning' | 'info';
  type: string;
  impact: string;
  fix: string;
  changes?: Array<{
    field: string;
    change: string;
    type: 'breaking' | 'warning' | 'info';
  }>;
}

const scenarios: Scenario[] = [
  {
    id: 'header-addition',
    title: 'Added Header: X-API-Version',
    description: 'Adding a version header helps clients identify the API version they are consuming.',
    severity: 'info',
    type: 'REST',
    impact: 'Low - Clients can optionally use the version header',
    fix: 'Document the header usage and maintain backward compatibility',
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
    changes: [
      { field: 'Response header', change: 'X-RateLimit-Remaining → removed', type: 'warning' }
    ]
  }
];

const ScenarioLibrary: React.FC = () => {
  const [activeFilter, setActiveFilter] = useState<string>('all');
  const [activeScenario, setActiveScenario] = useState<Scenario | null>(null);

  const filteredScenarios = activeFilter === 'all'
    ? scenarios
    : scenarios.filter(s => s.severity === activeFilter);

  useEffect(() => {
    // Simulate the showScenario event to ensure component is ready when shown
    const handleShowScenario = () => {
      // Component is already rendered; we just ensure it's visible
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

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Scenario Library
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Learn about API contract changes with pre-configured breaking change scenarios
        </p>
      </div>

      {/* Filters */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <div className="flex flex-wrap gap-2">
          <button
            className={`px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors ${activeFilter === 'all' ? 'active bg-accent-info/20 text-accent-info' : ''}`}
            data-filter="all"
            onClick={() => setActiveFilter('all')}
          >
            All Scenarios
          </button>
          <button
            className={`px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors ${activeFilter === 'breaking' ? 'active bg-accent-breaking/20 text-accent-breaking' : ''}`}
            data-filter="breaking"
            onClick={() => setActiveFilter('breaking')}
          >
            Breaking
          </button>
          <button
            className={`px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors ${activeFilter === 'warning' ? 'active bg-accent-warning/20 text-accent-warning' : ''}`}
            data-filter="warning"
            onClick={() => setActiveFilter('warning')}
          >
            Warnings
          </button>
          <button
            className={`px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors ${activeFilter === 'info' ? 'active bg-accent-healthy/20 text-accent-healthy' : ''}`}
            data-filter="info"
            onClick={() => setActiveFilter('info')}
          >
            Informational
          </button>
        </div>
      </div>

      {activeScenario ? (
        {/* Scenario Detail View */}
        <div className="bg-bg-card rounded-xl border border-border-color p-6">
          <div className="flex justify-between items-start mb-4">
            <div className="flex items-center space-x-3">
              <div className={`w-8 h-8 flex items-center justify-center rounded-lg
                ${activeScenario.severity === 'breaking' ? 'bg-accent-breaking/20'
                : activeScenario.severity === 'warning' ? 'bg-accent-warning/20'
                : 'bg-accent-healthy/20'}`}>
                <span className={`font-bold text-${activeScenario.severity === 'breaking' ? 'accent-breaking'
                : activeScenario.severity === 'warning' ? 'accent-warning'
                : 'accent-healthy'}`}>
                  {activeScenario.severity.toUpperCase()}
                </span>
              </div>
              <div>
                <h3 className="text-xl font-semibold text-text-main mb-1">
                  {activeScenario.title}
                </h3>
                <p className="text-sm text-text-muted">
                  {activeScenario.type}
                </p>
              </div>
            </div>
            <button
              onClick={() => {
                setActiveScenario(null);
                // Hide the scenario library and show the main view
                const scenarioRoot = document.getElementById('scenario-react-root');
                if (scenarioRoot) {
                  (scenarioRoot as HTMLElement).style.display = 'none';
                }
                const webhookRoot = document.getElementById('webhook-react-root');
                if (webhookRoot) {
                  (webhookRoot as HTMLElement).style.display = 'none';
                }
                // Show the traffic view by default
                document.getElementById('view-traffic')!.classList.add('active');
                document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
                document.getElementById('nav-traffic')!.classList.add('active');
              }}
              className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            >
              Back to Scenarios
            </button>
          </div>

          <div className="space-y-4">
            <p className="text-base text-text-muted">
              {activeScenario.description}
            </p>

            <div className="grid gap-4">
              <div>
                <h4 className="font-semibold text-text-main mb-2">
                  Impact
                </h4>
                <p className="text-base text-text-muted">
                  {activeScenario.impact}
                </p>
              </div>
              <div>
                <h4 className="font-semibold text-text-main mb-2">
                  Type
                </h4>
                <p className="text-base text-text-muted">
                  {activeScenario.type}
                </p>
              </div>
            </div>

            {activeScenario.changes && activeScenario.changes.length > 0 ? (
              <div className="mt-4">
                <h4 className="font-semibold text-text-main mb-2">
                  Changes
                </h4>
                <div className="space-y-2">
                  {activeScenario.changes.map((change, index) => (
                    <div key={index} className="bg-bg-hover rounded-lg p-3">
                      <div className="flex justify-between">
                        <span className="text-sm font-medium text-text-muted">
                          Changes:
                        </span>
                        <span className="font-mono text-text-main">
                          {change.change}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}

            <div className="mt-4">
              <h4 className="font-semibold text-text-main mb-2">
                Fix
              </h4>
              <p className="text-base text-text-muted">
                {activeScenario.fix}
              </p>
            </div>
          </div>
        </div>
      ) : (
        {/* Scenario Grid View */}
        <div className="grid gap-6">
          {filteredScenarios.map(scenario => (
            <div key={scenario.id} className="bg-bg-card rounded-xl border border-border-color p-6">
              <div className="flex justify-between items-start mb-4">
                <div className="flex items-center space-x-3">
                  <div className={`w-8 h-8 flex items-center justify-center rounded-lg
                    ${scenario.severity === 'breaking' ? 'bg-accent-breaking/20'
                    : scenario.severity === 'warning' ? 'bg-accent-warning/20'
                    : 'bg-accent-healthy/20'}`}>
                    <span className={`font-bold text-${scenario.severity === 'breaking' ? 'accent-breaking'
                    : scenario.severity === 'warning' ? 'accent-warning'
                    : 'accent-healthy'}`}>
                      {scenario.severity.toUpperCase()}
                    </span>
                  </div>
                  <div>
                    <h3 className="text-xl font-semibold text-text-main mb-1">
                      {scenario.title}
                    </h3>
                    <p className="text-sm text-text-muted">
                      {scenario.type}
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => {
                    setActiveScenario(scenario);
                                      }}
                  className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
                >
                  View Details
                </button>
              </div>

              <div className="space-y-4">
                <p className="text-base text-text-muted">
                  {scenario.description}
                </p>

                <div className="grid gap-4">
                  <div>
                    <h4 className="font-semibold text-text-main mb-2">
                      Impact
                    </h4>
                    <p className="text-base text-text-muted">
                      {scenario.impact}
                    </p>
                  </div>
                  <div>
                    <h4 className="font-semibold text-text-main mb-2">
                      Type
                    </h4>
                    <p className="text-base text-text-muted">
                      {scenario.type}
                    </p>
                  </div>
                </div>

                {scenario.changes && scenario.changes.length > 0 ? (
                  <div className="mt-4">
                    <h4 className="font-semibold text-text-main mb-2">
                      Changes
                    </h4>
                    <div className="space-y-2">
                      {scenario.changes.map((change, index) => (
                        <div key={index} className="bg-bg-hover rounded-lg p-3">
                          <div className="flex justify-between">
                            <span className="text-sm font-medium text-text-muted">
                              Changes:
                            </span>
                            <span className="font-mono text-text-main">
                              {change.change}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                : null}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default ScenarioLibrary;