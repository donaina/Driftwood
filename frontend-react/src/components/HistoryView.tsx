import React, { useState, useEffect } from 'react';
import EndpointHistory, { type HistoryItem } from './EndpointHistory';
import {
  Button,
  EmptyState,
  Panel,
  RefreshIcon,
  SkeletonRows,
  ViewHeader,
  toast,
} from './ui';

/* This used to redeclare the wire type locally, as `History` with PascalCase
   fields. One copy got corrected and the other did not, which is exactly how
   the field-name bug survived: the two only had to agree with each other,
   never with the API. There is one definition now, and it lives next to the
   component that consumes it. */

const HistoryView: React.FC = () => {
  const [histories, setHistories] = useState<HistoryItem[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
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
      const data: HistoryItem[] = await res.json();
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

  /* Pin or release an endpoint's contract version.

     A failure here gets its own banner rather than the page-level error state:
     that state replaces the whole view, so a lock that did not take would hide
     the history the user was reading in order to tell them the lock did not
     take. The version passed for a release is 0, which is what the API reads as
     "track the latest" — versions are numbered from 1, so 0 cannot collide with
     a real one. */
  const handleToggleLock = async (history: HistoryItem, version: number, lock: boolean) => {
    setActionError(null);
    try {
      const res = await fetch('/_driftwood/api/baselines/lock', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          method: history.method,
          path: history.path,
          version: lock ? version : 0,
        }),
      });
      if (!res.ok) {
        throw new Error(`${res.status} ${(await res.text()).trim()}`);
      }
      const updated: HistoryItem = await res.json();
      setHistories((prev) =>
        prev.map((h) =>
          h.method === updated.method && h.path === updated.path ? updated : h
        )
      );
    } catch (err) {
      console.error(err);
      setActionError(
        `Could not ${lock ? 'lock' : 'release'} ${history.method} ${history.path} at v${version}: ${err}`
      );
    }
  };

  /* Accept a captured response as the contract.

     This is the one control that can tell an already-drifted API apart from a
     healthy one, and it is a person's call rather than the program's: Driftwood
     has no way to know whether the first response it saw was correct. */
  const handleConfirm = async (history: HistoryItem, version: number) => {
    setActionError(null);
    try {
      const res = await fetch('/_driftwood/api/baselines/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          method: history.method,
          path: history.path,
          version,
        }),
      });
      if (!res.ok) {
        throw new Error(`${res.status} ${(await res.text()).trim()}`);
      }
      const updated: HistoryItem = await res.json();
      setHistories((prev) =>
        prev.map((h) =>
          h.method === updated.method && h.path === updated.path ? updated : h
        )
      );
    } catch (err) {
      console.error(err);
      setActionError(
        `Could not confirm ${history.method} ${history.path} v${version}: ${err}`
      );
    }
  };

  const handleExportTimeline = (format: string, endpointKey: string) => {
    // Placeholder for export functionality
    toast(
      `Export ${format.toUpperCase()}`,
      `Export for ${endpointKey} is planned for a future update.`
    );
    // In a full implementation, this would use html2canvas or similar library
    // to convert the timeline view to the requested format
  };

  const refresh = (
    <Button variant="primary" onClick={loadHistories}>
      <RefreshIcon />
      Refresh History
    </Button>
  );

  if (loading) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Version History Browser" align="center" action={refresh} />
        <SkeletonRows rows={4} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Version History Browser" align="center" action={refresh} />
        {/* role="alert" rather than a live region: this is the response to
            something the user just did, and it replaces the view. */}
        <Panel tone="error" className="text-center text-accent-breaking" role="alert">
          {error}
        </Panel>
      </div>
    );
  }

  if (histories.length === 0) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Version History Browser" align="center" action={refresh} />
        <EmptyState
          title="No baseline contracts yet"
          body={
            <>
              History is built from what the proxy observes, so it fills in as
              requests pass through the Driftwood proxy rather than from
              anything you do here. Point a client at the proxy address to
              record the first response.
            </>
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ViewHeader title="Version History Browser" action={refresh} />
      {actionError && (
        <Panel pad="sm" tone="error" className="text-accent-breaking" role="alert">
          {actionError}
        </Panel>
      )}
      <div className="space-y-6">
        {histories.map((history) => {
          const endpointKey = `${history.method}:${history.path}`;
          return (
            <EndpointHistory
              key={endpointKey}
              history={history}
              selectedVersionsMap={selectedVersionsMap}
              onToggleVersionSelection={handleToggleVersionSelection}
              onClearVersionSelection={handleClearVersionSelection}
              onExportTimeline={handleExportTimeline}
              onToggleLock={handleToggleLock}
              onConfirm={handleConfirm}
            />
          );
        })}
      </div>
    </div>
  );
};

export default HistoryView;