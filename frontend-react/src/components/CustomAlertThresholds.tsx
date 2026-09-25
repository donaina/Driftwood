import React, { useCallback, useEffect, useState } from 'react';
import { Button, Field, Panel, PanelTitle, ViewHeader, SkeletonRows, inputClass, toast } from './ui';

/* What raises an alert, per project.

   This screen used to collect five severities that nothing read. It toasted
   "Custom alert thresholds have been saved." over an empty function body, and
   the store raised an alert on the literal `HasBreakingChanges || HasWarnings`.
   The screen configured one thing and the engine did another, which is the
   shape of bug this file exists to remove.

   Two things changed with it. The five rows became seven, because the engine
   emits seven delta kinds and the old five did not partition them: "Added /
   Removed Fields" configured the same kind as "Required → Optional Fields"
   above it, and "Header Changes" configured no kind at all — headers are
   recorded on traffic and never diffed. And a row now sets a *floor* rather
   than a severity: the least severe change of that kind worth telling you
   about. It cannot relabel a change, because severity is a measurement.

   The floors come from the server, and so does what each preset means. This
   file holds the copy and nothing else: a preset name expanded here would be a
   second definition of it, and the two would drift. */

const BASE = '/_driftwood/api/thresholds';

type Severity = 'BREAKING' | 'WARNING' | 'INFO';

interface FloorRow {
  kind: string;
  floor: Severity;
}

interface Preset {
  name: string;
  floors: FloorRow[];
}

interface ThresholdsView {
  preset: string;
  floors: FloorRow[];
  presets: Preset[];
  updated_at?: string;
}

/* The engine's delta kinds, in the order the server sends them. The copy is
   here because it is user-facing; the order and the set are not, because a
   kind that goes missing from a config screen is a change nobody can
   configure. A kind with no entry falls back to the server's own identifier —
   ugly, and honest about what it does not know. */
const KIND_COPY: Record<string, { label: string; hint: string }> = {
  TYPE_MISMATCH: {
    label: 'Type changes',
    hint: 'a value changed type — an id that was a number is now a string',
  },
  REMOVED_FIELD: {
    label: 'Removed fields',
    hint: 'a property the response used to send is gone',
  },
  ADDED_FIELD: {
    label: 'Added fields',
    hint: 'the response sends a property the baseline did not have',
  },
  NULLABILITY_CHANGE: {
    label: 'Nullability changes',
    hint: 'a property that was never null returned null',
  },
  ARRAY_ITEM_MISMATCH: {
    label: 'Array item type changes',
    hint: 'the items in an array changed type',
  },
  FORMAT_CHANGE: {
    label: 'Format changes',
    hint: 'a value stopped (or started) looking like a date, uuid or email',
  },
  STATUS_CODE_CHANGE: {
    label: 'Status code changes',
    hint: 'an endpoint that answered 2xx answered an error instead',
  },
};

const SEVERITIES: Array<{ value: Severity; label: string }> = [
  { value: 'BREAKING', label: 'Breaking' },
  { value: 'WARNING', label: 'Warning' },
  { value: 'INFO', label: 'Info' },
];

const PRESET_COPY: Record<string, { label: string; hint: string }> = {
  strict: { label: 'Strict', hint: 'Alert on every change, informational ones included.' },
  recommended: { label: 'Recommended', hint: 'Alert on breaking changes and warnings.' },
  lenient: { label: 'Lenient', hint: 'Alert only on breaking changes.' },
};

function kindCopy(kind: string) {
  return KIND_COPY[kind] ?? { label: kind, hint: 'this build reports this change and has no description for it' };
}

/* The route answers a refusal with a sentence — "unknown delta kind", or the
   message naming both version numbers — and that sentence is the only thing
   that says what to fix. A bare status code would leave the operator reading
   "400" and guessing, so the body wins when there is one. */
async function failure(res: Response): Promise<string> {
  const body = (await res.text()).trim();
  return body !== '' ? body : `${res.status} ${res.statusText}`;
}

/* A project that has never saved a floor is sent Go's zero time, which arrives
   as "0001-01-01T00:00:00Z" and parses into a perfectly VALID Date — so the
   NaN guard below does not catch it, and the footer renders "Saved as
   Recommended, 1/1/1, 12:13:35 AM": a save that never happened, dated to the
   year one. Caught by its year rather than by string equality, because the same
   instant carries whatever offset the server's location gives it. */
function formatSaved(at?: string): string | null {
  if (!at) return null;
  const when = new Date(at);
  if (Number.isNaN(when.getTime())) return null;
  if (when.getFullYear() <= 1) return null;
  return when.toLocaleString();
}

const CustomAlertThresholds: React.FC = () => {
  const [view, setView] = useState<ThresholdsView | null>(null);
  const [draft, setDraft] = useState<Record<string, Severity>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(BASE);
      if (!res.ok) throw new Error(await failure(res));
      const loaded: ThresholdsView = await res.json();
      setView(loaded);
      setDraft(Object.fromEntries(loaded.floors.map((row) => [row.kind, row.floor])));
    } catch (err) {
      console.error(err);
      setError(`Could not load alert thresholds: ${err}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  /* Read on mount and only on mount — `mountView` reuses a container's existing
     React root and re-renders it, so the mount effect never runs a second time.
     Without this listener the screen would go on showing the previous project's
     floors under the new project's name, and its Save button would write them
     onto the new project. Alert Delivery shipped that way once, and the reason
     is the same here: the floors are project state. */
  useEffect(() => {
    const onProjectChanged = () => {
      void load();
    };
    window.addEventListener('driftwood:project-changed', onProjectChanged);
    return () => window.removeEventListener('driftwood:project-changed', onProjectChanged);
  }, [load]);

  const applyPreset = (preset: Preset) => {
    setDraft(Object.fromEntries(preset.floors.map((row) => [row.kind, row.floor])));
  };

  const setFloor = (kind: string, floor: Severity) => {
    setDraft((prev) => ({ ...prev, [kind]: floor }));
  };

  const dirty =
    view !== null &&
    view.floors.some((row) => draft[row.kind] !== undefined && draft[row.kind] !== row.floor);

  /* Which card is lit is read off the DRAFT, not off the saved view. The two
     differ the moment a preset is pressed — that fills the table and stores
     nothing — and a highlight following the saved state would leave "Recommended"
     looking selected over a table of Strict's floors. It compares against the
     floors the server sent with each preset rather than expanding a preset name
     here, so there is still exactly one definition of what "strict" means. */
  const draftPreset =
    view === null
      ? null
      : (view.presets.find((preset) =>
          preset.floors.every((row) => (draft[row.kind] ?? row.floor) === row.floor),
        )?.name ?? 'custom');

  const savedPreset = view === null ? null : (PRESET_COPY[view.preset]?.label ?? null);

  const save = async () => {
    if (!view) return;
    setSaving(true);
    setActionError(null);
    try {
      const floors = view.floors.map((row) => ({
        kind: row.kind,
        floor: draft[row.kind] ?? row.floor,
      }));
      const res = await fetch(BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ floors }),
      });
      if (!res.ok) throw new Error(await failure(res));

      const saved: ThresholdsView = await res.json();
      setView(saved);
      // Re-seeded from what was stored, so the form shows the floors the server
      // kept rather than the ones that were on screen when Save was pressed.
      setDraft(Object.fromEntries(saved.floors.map((row) => [row.kind, row.floor])));

      /* What was actually saved, not "saved". The floor that ignores the most is
         worth naming in the confirmation: an operator who has just set every row
         to Breaking has turned off most of this product, and a bare "saved"
         would let them believe otherwise.

         Counted rather than phrased as "at <preset> or above": the floors need
         not be a preset at all, and "raised at custom or above" is a sentence
         about nothing. A preset is named only when the saved floors really are
         one. */
      const alerting = saved.floors.filter((row) => row.floor !== 'BREAKING');
      const named = PRESET_COPY[saved.preset]?.label;
      toast(
        'Thresholds saved',
        alerting.length === 0
          ? 'Only breaking changes will raise an alert.'
          : `${named ? `${named} — ` : ''}alerts will be raised for ${alerting.length} of ${saved.floors.length} change types.`
      );
    } catch (err) {
      console.error(err);
      /* A failed save is its own line, not the page-level `error`. Replacing the
         view would hide the floors the operator was editing — and the ones in
         force, which are still the server's. */
      setActionError(`Could not save thresholds: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const reset = () => {
    if (!view) return;
    setDraft(Object.fromEntries(view.floors.map((row) => [row.kind, row.floor])));
    setActionError(null);
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Alert Thresholds" align="center" />
        <SkeletonRows rows={4} />
      </div>
    );
  }

  if (error || !view) {
    return (
      <div className="space-y-6">
        <ViewHeader title="Alert Thresholds" align="center" />
        <Panel tone="error" className="text-center text-accent-breaking" role="alert">
          {error ?? 'The thresholds could not be read.'}
        </Panel>
      </div>
    );
  }

  const savedAt = formatSaved(view.updated_at);

  return (
    <div className="space-y-6">
      <ViewHeader
        title="Alert Thresholds"
        lead="A floor per change type: the least severe difference of that kind that raises an alert. The severity itself is measured, not configured — this decides what reaches you, never what a change is called."
      />

      <Panel>
        <PanelTitle className="mb-4">Presets</PanelTitle>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {view.presets.map((preset) => {
            const copy = PRESET_COPY[preset.name] ?? { label: preset.name, hint: '' };
            const active = draftPreset === preset.name;
            return (
              <button
                key={preset.name}
                type="button"
                aria-pressed={active}
                className={`flex flex-col gap-2 p-4 rounded-sm border text-left transition-all duration-100 active:translate-y-px hover:bg-bg-hover ${
                  active ? 'border-accent-primary bg-accent-primary/10' : 'border-border-color'
                }`}
                onClick={() => applyPreset(preset)}
              >
                <PanelTitle level={4} size="base">
                  {copy.label}
                </PanelTitle>
                <p className="text-text-sm text-text-muted">{copy.hint}</p>
              </button>
            );
          })}

          {/* Not a button: Custom is what the floors amount to, not something to
              choose. Pressing it would have to mean "make them non-uniform",
              which is a thing no click can do.

              accent-primary, not accent-healthy, on the selected card: whichever
              preset is chosen is an interactive selection, and the healthy hue
              means the contract still matches its baseline — the rule §7 of
              MEDIUM_TERM_IMPROVEMENTS.md records and the one this component
              broke first. */}
          <div
            className={`flex flex-col gap-2 p-4 rounded-sm border ${
              draftPreset === 'custom' ? 'border-accent-primary bg-accent-primary/10' : 'border-border-color'
            }`}
          >
            <PanelTitle level={4} size="base">
              Custom
            </PanelTitle>
            <p className="text-text-sm text-text-muted">
              {draftPreset === 'custom'
                ? 'These floors are not one preset — they were set per change type below.'
                : 'Set change types individually below to move off a preset.'}
            </p>
          </div>
        </div>
        <p className="mt-4 text-text-sm text-text-muted">
          Choosing a preset fills the table below. Nothing is stored until you save.
        </p>
      </Panel>

      <Panel>
        <PanelTitle className="mb-2">Per-change-type floors</PanelTitle>
        <p className="mb-6 text-text-sm text-text-muted">
          Each row is the <em>minimum</em> severity that raises an alert for that kind of change. Setting a row
          to Breaking means only breaking changes of that kind are recorded as alerts.
        </p>
        <div className="grid gap-6">
          {view.floors.map((row) => {
            const copy = kindCopy(row.kind);
            const value = draft[row.kind] ?? row.floor;
            return (
              <Field key={row.kind} label={copy.label}>
                <select
                  value={value}
                  aria-label={`${copy.label} floor`}
                  onChange={(e) => setFloor(row.kind, e.target.value as Severity)}
                  className={inputClass}
                >
                  {SEVERITIES.map((s) => (
                    <option key={s.value} value={s.value}>
                      {s.label}
                    </option>
                  ))}
                </select>
                <p className="text-text-sm text-text-muted">{copy.hint}</p>
              </Field>
            );
          })}
        </div>
      </Panel>

      {actionError && (
        <Panel tone="error" className="text-sm text-accent-breaking" role="alert">
          {actionError}
        </Panel>
      )}

      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between sm:gap-4 pt-6">
        <p className="text-text-sm text-text-muted">
          {/* The saved preset is named here rather than on the card, so a draft
              that differs from what is stored reads as a difference: the card
              lights the draft, this line reports the store. */}
          {dirty
            ? 'You have unsaved changes.'
            : savedAt
              ? `Saved ${savedPreset ? `as ${savedPreset}, ` : ''}${savedAt}.`
              : 'These floors have never been changed, so Driftwood is using its defaults.'}
        </p>
        <div className="flex flex-col sm:flex-row sm:gap-4 mt-4 sm:mt-0">
          <Button className="flex-1 sm:w-auto" onClick={reset} disabled={!dirty || saving}>
            Discard changes
          </Button>
          <Button variant="primary" className="flex-1 sm:w-auto" onClick={save} busy={saving} disabled={!dirty}>
            Save floors
          </Button>
        </div>
      </div>
    </div>
  );
};

export default CustomAlertThresholds;
