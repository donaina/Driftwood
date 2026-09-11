import React from 'react';

const ExportReporting: React.FC = () => {
  // In a real implementation, this would fetch data from the backend or have hardcoded examples
  const reportHistory = [
    {
      id: 1,
      type: 'Summary Report',
      format: 'PDF',
      generated: '2026-09-10 14:30',
      size: '2.4 MB',
      status: 'Completed'
    },
    {
      id: 2,
      type: 'Detailed Timeline',
      format: 'PNG',
      generated: '2026-09-09 09:15',
      size: '1.8 MB',
      status: 'Completed'
    }
  ];

  const handleExportAsPNG = () => {
    alert('Export functionality for PNG format is planned for a future update.');
    // In a full implementation, this would use html2canvas or similar library
    // to convert the current view to PNG
  };

  const handleExportAsSVG = () => {
    alert('Export functionality for SVG format is planned for a future update.');
    // In a full implementation, this would convert the view to SVG
  };

  const handleExportSummaryPDF = () => {
    alert('Summary PDF export functionality is planned for a future update.');
  };

  const handleExportTimelinePDF = () => {
    alert('Timeline PDF export functionality is planned for a future update.');
  };

  const handleScheduleReports = () => {
    const frequency = 'weekly'; // In real implementation, get from form
    const recipients = 'team@example.com'; // In real implementation, get from form
    const reportType = 'both'; // In real implementation, get from form

    alert(`Scheduled ${frequency} ${reportType} reports sent to: ${recipients}`);
    // In a full implementation, this would set up actual scheduled reports via backend
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Export & Reporting
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Generate shareable reports of API contract stability for team communication
        </p>
        <div className="flex justify-center mt-4">
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={() => alert('Export options would be shown here in a full implementation.')}
          >
            Export Current View
          </button>
        </div>
      </div>

      {/* Export Options */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Export Options
        </h3>
        <div className="grid gap-6">
          {/* Current View */}
          <div className="bg-bg-hover rounded-xl border border-border-color p-4">
            <h4 className="font-semibold text-text-main mb-2">Current View</h4>
            <p className="text-text-sm text-text-muted mb-3">
              Export the currently visible dashboard as an image
            </p>
            <div className="flex space-x-3">
              <button
                onClick={handleExportAsPNG}
                className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors flex-1"
              >
                Export as PNG
              </button>
              <button
                onClick={handleExportAsSVG}
                className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors flex-1"
              >
                Export as SVG
              </button>
            </div>
          </div>

          {/* Summary Report */}
          <div className="bg-bg-hover rounded-xl border border-border-color p-4">
            <h4 className="font-semibold text-text-main mb-2">Summary Report</h4>
            <p className="text-text-sm text-text-muted mb-3">
              Generate a PDF summary of contract health metrics
            </p>
            <button
              onClick={handleExportSummaryPDF}
              className="w-full px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            >
              Export Summary PDF
            </button>
          </div>

          {/* Detailed Timeline */}
          <div className="bg-bg-hover rounded-xl border border-border-color p-4">
            <h4 className="font-semibold text-text-main mb-2">Detailed Timeline</h4>
            <p className="text-text-sm text-text-muted mb-3">
              Export the full contract evolution timeline
            </p>
            <button
              onClick={handleExportTimelinePDF}
              className="w-full px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            >
              Export Timeline PDF
            </button>
          </div>
        </div>
      </div>

      {/* Scheduled Reports */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Scheduled Reports
        </h3>
        <p className="text-text-sm text-text-muted mb-4">
          Set up automatic email reports for regular stakeholder updates
        </p>
        <div className="grid gap-4">
          <div>
            <label className="block text-sm font-medium text-text-muted mb-2">
              Frequency:
            </label>
            <select
              className="w-full px-4 py-2 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="daily">Daily</option>
              <option value="weekly" selected>
                Weekly
              </option>
              <option value="monthly">Monthly</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-text-muted mb-2">
              Recipients:
            </label>
            <input
              type="email"
              placeholder="team@example.com, manager@company.com"
              multiple
              className="w-full px-4 py-2 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-text-muted mb-2">
              Report Type:
            </label>
            <select
              className="w-full px-4 py-2 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="summary">Summary Report</option>
              <option value="detailed">Detailed Timeline</option>
              <option value="both" selected>
                Both Summary & Timeline
              </option>
            </select>
          </div>
        </div>
        <div className="flex justify-end mt-4">
          <button
            onClick={handleScheduleReports}
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
          >
            Schedule Reports
          </button>
        </div>
      </div>

      {/* Report History */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Report History
        </h3>
        <p className="text-text-sm text-text-muted mb-4">
          View and manage previously generated reports
        </p>
        {reportHistory.length > 0 ? (
          <div className="space-y-4">
            {reportHistory.map(report => (
              <div key={report.id} className="flex justify-between items-center p-4 rounded-lg border border-border-color bg-bg-hover">
                <div>
                  <div className="font-semibold text-text-main">{report.type}</div>
                  <div className="text-xs text-text-muted">
                    {report.format} • {report.generated}
                  </div>
                </div>
                <div className="flex space-x-3 text-xs text-text-muted">
                  <span>{report.size}</span>
                  <span
                    className={`px-2 py-0.5 rounded-full text-xs font-medium
                    ${report.status.toLowerCase() === 'completed' ? 'bg-accent-healthy/20 text-accent-healthy'
                      : 'bg-border-color/20 text-text-muted'}`}
                  >
                    {report.status}
                  </span>
                </div>
              }
            ))}
          </div>
        ) : (
          <div className="text-center py-8">
            <p className="text-text-muted">
              No reports generated yet
            </p>
          </div>
        )}
      </div>
    </div>
  );
};

export default ExportReporting;