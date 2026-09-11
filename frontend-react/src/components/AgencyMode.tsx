import React from 'react';

const AgencyMode: React.FC = () => {
  // Mock data for agencies/clients
  const agencies = [
    {
      id: 1,
      name: 'Acme Corp',
      status: 'active',
      endpoints: 12,
      alerts: 3,
      lastChecked: '2026-09-10 14:30',
    },
    {
      id: 2,
      name: 'Beta Industries',
      status: 'active',
      endpoints: 8,
      alerts: 0,
      lastChecked: '2026-09-10 12:15',
    },
    {
      id: 3,
      name: 'Gamma LLC',
      status: 'warning',
      endpoints: 5,
      alerts: 2,
      lastChecked: '2026-09-10 09:45',
    },
    {
      id: 4,
      name: 'Delta Enterprises',
      status: 'inactive',
      endpoints: 0,
      alerts: 0,
      lastChecked: '2026-09-09 16:20',
    },
  ];

  const handleAddAgency = () => {
    // In a real implementation, this would show a modal/form
    alert('Add agency functionality would be shown here in a full implementation.');
  };

  const handleViewDetails = (agencyId: number) => {
    // In a real implementation, this would show detailed view
    alert(`Viewing details for agency ${agencyId}`);
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Multi-Tenant View (Agency Mode)
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Manage multiple client accounts and their API contract monitoring from a single dashboard.
        </p>
        <div className="flex justify-center mt-4">
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={handleAddAgency}
          >
            Add Agency
          </button>
        </div>
      </div>

      {/* Agency Overview */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Agency Overview
        </h3>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {/* Total Agencies */}
          <div className="flex flex-col items-center space-y-3 p-4 rounded-lg border border-border-color">
            <h4 className="text-sm font-medium text-text-muted">Total Agencies</h4>
            <p className="text-2xl font-bold text-text-main">{agencies.length}</p>
          </div>

          {/* Active Agencies */}
          <div className="flex flex-col items-center space-y-3 p-4 rounded-lg border border-border-color">
            <h4 className="text-sm font-medium text-text-muted">Active Agencies</h4>
            <p className="text-2xl font-bold text-accent-healthy">
              {agencies.filter(a => a.status === 'active').length}
            </p>
          </div>

          {/* Total Endpoints */}
          <div className="flex flex-col items-center space-y-3 p-4 rounded-lg border border-border-color">
            <h4 className="text-sm font-medium text-text-muted">Total Endpoints</h4>
            <p className="text-2xl font-bold text-text-main">
              {agencies.reduce((sum, agency) => sum + agency.endpoints, 0)}
            </p>
          </div>

          {/* Active Alerts */}
          <div className="flex flex-col items-center space-y-3 p-4 rounded-lg border border-border-color">
            <h4 className="text-sm font-medium text-text-muted">Active Alerts</h4>
            <p className="text-2xl font-bold text-accent-breaking">
              {agencies.reduce((sum, agency) => sum + agency.alerts, 0)}
            </p>
          </div>
        </div>
      </div>

      {/* Managed Agencies List */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Managed Agencies
        </h3>
        <div className="space-y-4">
          {/* Table Header */}
          <div className="grid grid-cols-6 gap-4 pb-3 border-b border-border-color">
            <div className="col-span-2 text-sm font-medium text-text-muted uppercase letter-spacing-wider">
              Agency Name
            </div>
            <div className="col-span-1 text-sm font-medium text-text-muted uppercase letter-spacing-wider">
              Endpoints
            </div>
            <div className="col-span-1 text-sm font-medium text-text-muted uppercase letter-spacing-wider">
              Alerts
            </div>
            <div className="col-span-1 text-sm font-medium text-text-muted uppercase letter-spacing-wider">
              Status
            </div>
            <div className="col-span-1 text-sm font-medium text-text-muted uppercase letter-spacing-wider">
              Last Checked
            </div>
          </div>

          {/* Agency Items */}
          {agencies.length > 0 ? (
            agencies.map(agency => (
              <div key={agency.id} className="grid grid-cols-6 gap-4 py-4 border-t border-border-color last:border-0">
                <div className="col-span-2 flex items-center space-x-3">
                  <div className="w-8 h-8 flex items-center justify-center rounded-lg bg-bg-hover">
                    <span className="font-bold text-text-main">{agency.name.charAt(0)}</span>
                  </div>
                  <div>
                    <p className="font-medium text-text-main">{agency.name}</p>
                    <p className="text-xs text-text-muted">
                      {agency.name.toLowerCase().replace(/\s/g, '')}.api.example.com
                    </p>
                  </div>
                </div>
                <div className="col-span-1 text-center font-medium text-text-main">
                  {agency.endpoints}
                </div>
                <div className="col-span-1 flex items-center justify-center">
                  <span
                    className={`px-2 py-0.5 rounded-full text-xs font-medium
                    ${agency.alerts > 0 ? 'bg-accent-breaking/20 text-accent-breaking' : 'bg-accent-info/20 text-accent-info'}`}
                  >
                    {agency.alerts}
                  </span>
                </div>
                <div className="col-span-1 flex items-center justify-center">
                  <span
                    className={`px-2 py-0.5 rounded-full text-xs font-medium
                    ${agency.status === 'active' ? 'bg-accent-healthy/20 text-accent-healthy'
                    : agency.status === 'warning' ? 'bg-accent-warning/20 text-accent-warning'
                    : 'bg-border-color/20 text-text-muted'}`}
                  >
                    {agency.status.charAt(0).toUpperCase() + agency.status.slice(1)}
                  </span>
                </div>
                <div className="col-span-1 text-center text-sm text-text-muted">
                  {agency.lastChecked}
                </div>
                <div className="col-span-1 flex items-center justify-center">
                  <button
                    className="px-3 py-1.5 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
                    onClick={() => handleViewDetails(agency.id)}
                  >
                    View
                  </button>
                </div>
              </div>
            ))
          ) : (
            <div className="text-center py-8">
              <p className="text-text-muted">No agencies added yet. Click "Add Agency" to get started.</p>
            </div>
          )}
        </div>
      </div>

      {/* Agency Actions */}
      <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4 pt-6">
        <button
          onClick={() => alert('Global settings would be configured here.')}
          className="px-6 py-3 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors flex-1 sm:auto"
        >
          Global Settings
        </button>
        <button
          onClick={() => alert('Bulk operations would be shown here.')}
          className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors flex-1 sm:auto"
        >
          Bulk Operations
        </button>
      </div>
    </div>
  );
};

export default AgencyMode;