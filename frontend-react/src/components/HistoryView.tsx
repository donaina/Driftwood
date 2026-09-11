import React, { useState, useEffect } from 'react';
import EndpointHistory from './EndpointHistory';

interface History {
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

const HistoryView: React.FC = () => {
  const [histories, setHistories] = useState<History[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedVersionsMap, setSelectedVersionsMap] = useState<Map<string, number[]>>(new Map());

  useEffect(() => {
    loadHistories();
  }, []);

  const loadHistories = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch('/_driftwood/api/histories');
      if (!res.ok) {
        throw new Error(`Failed to fetch histories: ${res.status}`);
      }
      const data: History[] = await res.json();
      setHistories(data);
    } catch (err) {
      console.error(err);
      setError('Failed to load history data');
    } finally {
      setLoading(false);
    }
  };

  const handleToggleVersionSelection = (endpointKey: string, version: number) => {
    setSelectedVersionsMap((prevMap) => {
      const newMap = new Map(prevMap);
      const selectedVersions = newMap.get(endpointKey) || [];
      const index = selectedVersions.indexOf(version);
      if (index > -1) {
        // Remove if already selected
        selectedVersions.splice(index, 1);
      } else {
        // Add if not selected, but limit to 2
        if (selectedVersions.length >= 2) {
          // Remove the oldest selection (first one)
          selectedVersions.shift();
        }
        selectedVersions.push(version);
      }
      newMap.set(endpointKey, selectedVersions);
      return newMap;
    });
  };

  const handleClearVersionSelection = (endpointKey: string) => {
    setSelectedVersionsMap((prevMap) => {
      const newMap = new Map(prevMap);
      newMap.delete(endpointKey);
      return newMap;
    });
  };

  const handleExportTimeline = (format: string, endpointKey: string) => {
    // Placeholder for export functionality
    alert(`Export functionality for ${format.toUpperCase()} format is planned for a future update.`);
    // In a full implementation, this would use html2canvas or similar library
    // to convert the timeline view to the requested format
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <h2 className="text-3xl font-bold text-text-main mb-4">
            Version History Browser
          </h2>
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={loadHistories}
          >
            🔄 Refresh History
          </button>
        </div>
        <div className="flex justify-center">
          <div className="animate-pulse h-8 w-8 rounded-full bg-accent-info/20"></div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <h2 className="text-3xl font-bold text-text-main mb-4">
            Version History Browser
          </h2>
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={loadHistories}
          >
            🔄 Refresh History
          </button>
        </div>
        <div className="bg-bg-card rounded-xl border border-border-color p-6 text-center">
          <p className="text-text-muted">{error}</p>
        </div>
      </div>
    );
  }

  if (histories.length === 0) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <h2 className="text-3xl font-bold text-text-main mb-4">
            Version History Browser
          </h2>
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={loadHistories}
          >
            🔄 Refresh History
          </button>
        </div>
        <div className="bg-bg-card rounded-xl border border-border-color p-6 text-center">
          <p className="text-text-muted">
            No baseline contracts found. History will appear as you track API traffic.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-3xl font-bold text-text-main">
          Version History Browser
        </h2>
        <button
          className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
          onClick={loadHistories}
        >
          🔄 Refresh History
        </button>
      </div>
      <div className="space-y-6">
        {histories.map((history) => {
          const endpointKey = `${history.Method}:${history.Path}`;
          return (
            <EndpointHistory
              key={endpointKey}
              history={history}
              selectedVersionsMap={selectedVersionsMap}
              onToggleVersionSelection={handleToggleVersionSelection}
              onClearVersionSelection={handleClearVersionSelection}
              onExportTimeline={handleExportTimeline}
            />
          );
        })}
      </div>
    </div>
  );
};

export default HistoryView;