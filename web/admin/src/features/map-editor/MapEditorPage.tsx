import { ApiProblem } from '@arenile/shared/api';
import type { AlignMode, Point } from '@arenile/shared/map';
import { Button, Card } from '@arenile/shared/ui';
import { Link, useParams } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  components as componentsResource,
  zones as zonesResource,
} from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, QueryState } from '../configuration/components/ConfigPage';
import { FormDialog } from '../configuration/components/dialogs';
import { localize } from '../configuration/lib/i18nText';
import { problemMessageKey } from '../../lib/problem';
import { MapSaveProblem, useMapDocument, useSaveMap } from './api';
import { ConflictDialog, type ConflictInfo } from './ConflictDialog';
import { MapSettingsForm, RenumberRowForm, RowGeneratorForm, type ZoneOption } from './dialogs';
import { MapCanvas } from './MapCanvas';
import { draftFromApi, toSaveRequest, validateDraft } from './model/document';
import {
  addDecoration,
  addSpots,
  alignElements,
  allKeys,
  commitFrom,
  diffDocuments,
  distributeElements,
  duplicateElements,
  elements,
  freePosition,
  initHistory,
  moveElements,
  newDecoration,
  newSpot,
  pushHistory,
  redo,
  removeElements,
  renumberRow,
  replacePresent,
  rotateElements,
  toggleSelection,
  undo,
  updateDecoration,
  updateMapSettings,
  updateSpots,
  type History,
} from './model/editor';
import {
  isSpot,
  type ApiMapDocument,
  type DecorationKind,
  type DraftDocument,
  type DraftSpot,
  type Selection,
  type SpotKind,
} from './model/types';
import { ZONE_PALETTE } from './model/zoneColors';
import { Palette } from './Palette';
import { SelectionPanel, type ComponentOption } from './SelectionPanel';

type SaveState =
  | { kind: 'idle' }
  | { kind: 'saved' }
  | { kind: 'error'; messages: string[] }
  | { kind: 'allocated'; labels: string[] };

type DialogKind = 'generator' | 'settings' | 'renumber' | null;

/** Map editor (spec 11.2): drag & drop, row generator, versioned transactional save. */
export function MapEditorPage() {
  const { mapId } = useParams({ strict: false }) as { mapId: string };
  const ctx = useConfigContext();
  const { t } = useTranslation('mapEditor');
  const docQuery = useMapDocument(ctx.establishmentId, mapId);
  const zoneList = zonesResource.useList(ctx.establishmentId);
  const componentList = componentsResource.useList(ctx.establishmentId);
  // Bumped after an explicit reload: remounts the editor on the server version.
  const [generation, setGeneration] = useState(0);
  const fetchLatest = async (): Promise<ApiMapDocument> => {
    const r = await docQuery.refetch({ throwOnError: true });
    if (!r.data) throw new Error('map document missing');
    return r.data;
  };
  const components: ComponentOption[] = useMemo(
    () =>
      (componentList.data ?? []).map((c) => ({
        code: c.code,
        name: localize(c.name, ctx.uiLocale, ctx.defaultLocale) || c.code,
      })),
    [componentList.data, ctx.uiLocale, ctx.defaultLocale],
  );

  return (
    <ConfigPage
      title={
        docQuery.data
          ? t('editor.titleNamed', {
              name: localize(docQuery.data.map.name, ctx.uiLocale, ctx.defaultLocale),
            })
          : t('editor.title')
      }
      description={t('editor.description')}
      readOnly={!ctx.isLoading && !ctx.canConfigure}
      actions={
        <Button asChild variant="outline">
          <Link to="/e/$establishmentId/maps" params={{ establishmentId: ctx.establishmentId }}>
            {t('editor.backToList')}
          </Link>
        </Button>
      }
    >
      <QueryState
        isLoading={docQuery.isPending || zoneList.isPending}
        error={docQuery.error ?? zoneList.error}
        onRetry={() => void docQuery.refetch()}
      />
      {docQuery.data && zoneList.data && (
        <Editor
          key={`${docQuery.data.map.id}:${generation}`}
          serverDoc={docQuery.data}
          zones={zoneList.data}
          components={components}
          establishmentId={ctx.establishmentId}
          mapId={mapId}
          readOnly={!ctx.canConfigure}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          uiLocale={ctx.uiLocale}
          fetchLatest={fetchLatest}
          onReload={() => void fetchLatest().then(() => setGeneration((g) => g + 1))}
        />
      )}
    </ConfigPage>
  );
}

interface EditorProps {
  serverDoc: ApiMapDocument;
  zones: {
    id: string;
    code: string;
    name: Record<string, string>;
    map_id?: string;
    assignment_mode: string;
  }[];
  components: ComponentOption[];
  establishmentId: string;
  mapId: string;
  readOnly: boolean;
  locales: string[];
  defaultLocale: string;
  uiLocale: string;
  /** Re-reads the server document (conflict diff, "overwrite"). */
  fetchLatest: () => Promise<ApiMapDocument>;
  /** Discards the draft and reloads the server version. */
  onReload: () => void;
}

function isEditableTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || typeof el.closest !== 'function') return false;
  return !!el.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]');
}

function Editor(props: EditorProps) {
  const { serverDoc, establishmentId, mapId, readOnly, defaultLocale, uiLocale } = props;
  const { t } = useTranslation('mapEditor');
  const save = useSaveMap(establishmentId, mapId);
  const [history, setHistory] = useState<History>(() => initHistory(draftFromApi(serverDoc)));
  const [version, setVersion] = useState(serverDoc.map.version);
  // Last loaded or saved version: the base of "your changes" in the conflict dialog.
  const [savedDoc, setSavedDoc] = useState<DraftDocument>(history.present);
  const [selection, setSelection] = useState<Selection>(new Set());
  const [saveState, setSaveState] = useState<SaveState>({ kind: 'idle' });
  const [conflict, setConflict] = useState<ConflictInfo | null>(null);
  const [overwriting, setOverwriting] = useState(false);
  const [dialog, setDialog] = useState<DialogKind>(null);
  const doc = history.present;
  const dirty = doc !== savedDoc;

  // Zones that may hold spots of this map: seat/zone zones of this map or without a map.
  const zoneOptions: ZoneOption[] = useMemo(
    () =>
      props.zones
        .filter((z) => z.assignment_mode !== 'quantity')
        .map((z) => ({
          id: z.id,
          name: localize(z.name, uiLocale, defaultLocale) || z.code,
          disabled: !!z.map_id && z.map_id !== mapId,
        })),
    [props.zones, uiLocale, defaultLocale, mapId],
  );
  const zoneName = useCallback(
    (id: string) => zoneOptions.find((z) => z.id === id)?.name ?? t('panel.noZone'),
    [zoneOptions, t],
  );
  const zoneColor = useCallback(
    (id: string) => {
      const i = props.zones.findIndex((z) => z.id === id);
      return i < 0 ? '#94a3b8' : (ZONE_PALETTE[i % ZONE_PALETTE.length] as string);
    },
    [props.zones],
  );

  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const commit = (next: DraftDocument) => {
    setHistory((h) => pushHistory(h, next));
    if (saveState.kind !== 'idle') setSaveState({ kind: 'idle' });
  };

  const selected = elements(doc).filter((el) => selection.has(el.key));
  const selectedSpots = selected.filter(isSpot);
  const issues = validateDraft(doc);
  const firstZone = zoneOptions.find((z) => !z.disabled)?.id ?? '';

  const labelsOf = (ids: string[], ...docs: DraftDocument[]) =>
    ids.map((id) => {
      for (const d of docs) {
        const s = d.spots.find((x) => x.id === id);
        if (s) return s.label;
      }
      return id;
    });

  const saveWith = async (baseVersion: number, knownIds?: Parameters<typeof toSaveRequest>[1]) => {
    try {
      const saved = await save.mutateAsync({
        version: baseVersion,
        body: toSaveRequest(doc, knownIds),
      });
      const next = draftFromApi(saved);
      setHistory(initHistory(next));
      setSavedDoc(next);
      setVersion(saved.map.version);
      setSelection(new Set());
      setConflict(null);
      setSaveState({ kind: 'saved' });
    } catch (err) {
      await onSaveError(err);
    }
  };

  const onSaveError = async (err: unknown) => {
    if (err instanceof ApiProblem && (err.status === 412 || err.type === 'version_conflict')) {
      const currentVersion = err instanceof MapSaveProblem ? err.currentVersion : undefined;
      const yours = diffDocuments(savedDoc, doc);
      setConflict({ currentVersion, yours, theirs: null });
      // Show what was saved meanwhile; the dialog works without it too.
      try {
        const latest = await props.fetchLatest();
        setConflict({
          currentVersion: latest.map.version,
          yours,
          theirs: diffDocuments(savedDoc, draftFromApi(latest)),
        });
      } catch {
        /* keep the dialog without the server side */
      }
      return;
    }
    setConflict(null);
    if (err instanceof MapSaveProblem && err.type === 'spot_has_future_allocations') {
      // Spots removed from the draft exist only in the saved version.
      setSaveState({ kind: 'allocated', labels: labelsOf(err.spotIds, savedDoc, doc) });
    } else if (err instanceof ApiProblem && err.fieldErrors.length > 0) {
      setSaveState({
        kind: 'error',
        messages: err.fieldErrors.map((f) => (f.field ? `${f.field}: ${f.message}` : f.message)),
      });
    } else if (err instanceof ApiProblem && err.detail) {
      setSaveState({ kind: 'error', messages: [err.detail] });
    } else {
      setSaveState({
        kind: 'error',
        messages: [t(problemMessageKey(err), { ns: 'translation' })],
      });
    }
  };

  const onSave = async () => {
    if (issues.length > 0 || readOnly) return;
    setSaveState({ kind: 'idle' });
    await saveWith(version);
  };

  /** Saves the draft on top of the server's current version (after a conflict). */
  const onOverwrite = async () => {
    setOverwriting(true);
    try {
      // Keep the ids the server still has; re-create elements it deleted meanwhile.
      const fresh = await props.fetchLatest();
      await saveWith(fresh.map.version, {
        spots: new Set(fresh.spots.map((s) => s.id)),
        decorations: new Set(fresh.decorations.map((d) => d.id)),
      });
    } catch (err) {
      setConflict(null);
      setSaveState({ kind: 'error', messages: [t(problemMessageKey(err), { ns: 'translation' })] });
    } finally {
      setOverwriting(false);
    }
  };

  const onSelect = useCallback((key: string, additive: boolean) => {
    setSelection((prev) => toggleSelection(prev, key, additive));
  }, []);

  const onMove = (keys: Selection, delta: Point, anchorKey: string) => {
    // The canvas already snapped the delta on the anchor: one undo step.
    const base = doc;
    setHistory((h) =>
      commitFrom(replacePresent(h, moveElements(base, keys, delta, { anchorKey })), base),
    );
    if (saveState.kind !== 'idle') setSaveState({ kind: 'idle' });
  };

  // Keyboard commands act on the selection, or on the focused element alone.
  const targetOf = (key: string): Selection => (selection.has(key) ? selection : new Set([key]));

  const center = (): Point => ({
    x: Math.round(doc.map.width / 2),
    y: Math.round(doc.map.height / 2),
  });

  const addSpotOfKind = (kind: SpotKind) => {
    if (!firstZone) return;
    // New spots go in the zone of the selected spot, if any.
    const zone =
      selectedSpots.find((s) => zoneOptions.some((z) => z.id === s.zone_id && !z.disabled))
        ?.zone_id ?? firstZone;
    const s = newSpot(doc, zone, { kind, at: freePosition(doc, center()) });
    commit(addSpots(doc, [s]));
    setSelection(new Set([s.key]));
  };

  const addDecorationOfKind = (kind: DecorationKind) => {
    const d = newDecoration(doc, kind, freePosition(doc, center()));
    commit(addDecoration(doc, d));
    setSelection(new Set([d.key]));
  };

  const duplicate = () => {
    const g = doc.map.grid_size > 0 ? doc.map.grid_size : 10;
    const r = duplicateElements(doc, selection, { x: g * 2, y: g * 2 });
    if (r.keys.size === 0) return;
    commit(r.doc);
    setSelection(r.keys);
  };

  const removeSelection = (keys: Selection = selection) => {
    commit(removeElements(doc, keys));
    setSelection(new Set());
  };

  // Global shortcuts (spec 11.2): undo/redo, duplicate, select all, deselect.
  const shortcuts = useRef<(e: KeyboardEvent) => void>(() => undefined);
  shortcuts.current = (e: KeyboardEvent) => {
    if (dialog || conflict || isEditableTarget(e.target)) return;
    const mod = e.ctrlKey || e.metaKey;
    const k = e.key.toLowerCase();
    if (mod && k === 'z') {
      e.preventDefault();
      setHistory(e.shiftKey ? redo : undo);
    } else if (mod && k === 'y') {
      e.preventDefault();
      setHistory(redo);
    } else if (mod && k === 'd' && !readOnly) {
      e.preventDefault();
      duplicate();
    } else if (mod && k === 'a') {
      e.preventDefault();
      setSelection(allKeys(doc));
    } else if (mod && k === 's' && !readOnly) {
      e.preventDefault();
      if (dirty && !save.isPending) void onSave();
    } else if (e.key === 'Escape' && selection.size > 0) {
      setSelection(new Set());
    }
  };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => shortcuts.current(e);
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const spotLabel = useCallback(
    (s: DraftSpot) =>
      t('canvas.spot', {
        label: s.label,
        zone: zoneName(s.zone_id),
        state: s.active ? '' : t('canvas.inactive'),
      }).trim(),
    [t, zoneName],
  );

  return (
    <div className="space-y-4">
      <div
        className="flex flex-wrap items-center gap-2"
        role="toolbar"
        aria-label={t('tools.label')}
      >
        <Button
          onClick={() => void onSave()}
          aria-keyshortcuts="Control+S Meta+S"
          disabled={readOnly || !dirty || save.isPending || issues.length > 0}
        >
          {save.isPending ? t('common.actions.saving', { ns: 'translation' }) : t('tools.save')}
        </Button>
        <Button
          variant="outline"
          aria-keyshortcuts="Control+Z Meta+Z"
          onClick={() => setHistory(undo)}
          disabled={history.past.length === 0}
        >
          {t('tools.undo')}
        </Button>
        <Button
          variant="outline"
          aria-keyshortcuts="Control+Shift+Z Meta+Shift+Z"
          onClick={() => setHistory(redo)}
          disabled={history.future.length === 0}
        >
          {t('tools.redo')}
        </Button>
        {!readOnly && (
          <>
            <Button
              variant="secondary"
              disabled={!firstZone}
              onClick={() => setDialog('generator')}
            >
              {t('tools.generator')}
            </Button>
            <Button variant="outline" onClick={() => setDialog('settings')}>
              {t('tools.settings')}
            </Button>
          </>
        )}
        <span className="ml-auto text-sm text-slate-600" aria-live="polite">
          {t('editor.count', { count: doc.spots.length })}
          {dirty ? ` · ${t('editor.unsaved')}` : ''}
          {saveState.kind === 'saved' ? ` · ${t('editor.saved')}` : ''}
        </span>
      </div>
      {!readOnly && (
        <Palette
          canAddSpots={!!firstZone}
          onAddSpot={addSpotOfKind}
          onAddDecoration={addDecorationOfKind}
        />
      )}
      {!firstZone && !readOnly && (
        <p role="note" className="text-sm text-amber-800">
          {t('editor.noZones')}
        </p>
      )}
      {issues.length > 0 && (
        <ul role="alert" className="list-inside list-disc text-sm text-red-700">
          {issues.map((i) => (
            <li key={`${i.type}:${i.keys.join(',')}`}>
              {t(`issues.${i.type}`, { count: i.keys.length, label: i.label ?? '' })}
            </li>
          ))}
        </ul>
      )}
      {saveState.kind === 'allocated' && (
        <div
          role="alert"
          className="space-y-1 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800"
        >
          <p>{t('editor.allocated', { count: saveState.labels.length })}</p>
          <p className="font-medium">{saveState.labels.join(', ')}</p>
          <p>{t('panel.deactivateHint')}</p>
        </div>
      )}
      {saveState.kind === 'error' && (
        <ul
          role="alert"
          className="list-inside list-disc rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800"
        >
          {saveState.messages.map((m) => (
            <li key={m}>{m}</li>
          ))}
        </ul>
      )}
      <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <MapCanvas
          doc={doc}
          selection={selection}
          readOnly={readOnly}
          zoneColor={zoneColor}
          spotLabel={spotLabel}
          decorationLabel={(d) =>
            localize(d.label, uiLocale, defaultLocale) || t(`decorations.${d.kind}`)
          }
          onSelect={onSelect}
          onSelectKeys={setSelection}
          onClearSelection={() => setSelection(new Set())}
          onMove={onMove}
          onNudge={(delta, key) => commit(moveElements(doc, targetOf(key), delta))}
          onDelete={(key) => removeSelection(targetOf(key))}
        />
        <Card className="h-fit p-4">
          <h2 className="mb-3 text-base font-semibold">{t('panel.title')}</h2>
          <SelectionPanel
            selected={selected}
            zones={zoneOptions}
            components={props.components}
            defaultLocale={defaultLocale}
            readOnly={readOnly}
            onSpotPatch={(patch) => commit(updateSpots(doc, selection, patch))}
            onDecorationPatch={(key, patch) => commit(updateDecoration(doc, key, patch))}
            onAlign={(mode: AlignMode) => commit(alignElements(doc, selection, mode))}
            onDistribute={(axis) => commit(distributeElements(doc, selection, axis))}
            onRotate={(deg) => commit(rotateElements(doc, selection, deg))}
            onDuplicate={duplicate}
            onRenumber={() => setDialog('renumber')}
            onDelete={() => removeSelection()}
          />
          <p className="mt-4 text-xs text-slate-600">{t('panel.keyboardHint')}</p>
        </Card>
      </div>
      <FormDialog
        open={dialog === 'generator'}
        onOpenChange={(o) => setDialog(o ? 'generator' : null)}
        wide
        title={t('generator.title')}
        description={t('generator.description')}
      >
        {dialog === 'generator' && (
          <RowGeneratorForm
            establishmentId={establishmentId}
            mapId={mapId}
            doc={doc}
            zones={zoneOptions}
            onCancel={() => setDialog(null)}
            onAdd={(spots) => {
              commit(addSpots(doc, spots));
              setSelection(new Set(spots.map((s) => s.key)));
              setDialog(null);
            }}
          />
        )}
      </FormDialog>
      <FormDialog
        open={dialog === 'renumber'}
        onOpenChange={(o) => setDialog(o ? 'renumber' : null)}
        title={t('renumber.title')}
        description={t('renumber.description')}
      >
        {dialog === 'renumber' && (
          <RenumberRowForm
            spots={selectedSpots}
            onCancel={() => setDialog(null)}
            onSubmit={(opts) => {
              commit(renumberRow(doc, selection, opts));
              setDialog(null);
            }}
          />
        )}
      </FormDialog>
      <FormDialog
        open={dialog === 'settings'}
        onOpenChange={(o) => setDialog(o ? 'settings' : null)}
        wide
        title={t('tools.settings')}
      >
        {dialog === 'settings' && (
          <MapSettingsForm
            initial={doc.map}
            locales={props.locales}
            defaultLocale={defaultLocale}
            submitLabel={t('settings.apply')}
            onCancel={() => setDialog(null)}
            onSubmit={(v) => {
              commit(updateMapSettings(doc, v));
              setDialog(null);
            }}
          />
        )}
      </FormDialog>
      <ConflictDialog
        conflict={conflict}
        busy={overwriting || save.isPending}
        onClose={() => setConflict(null)}
        onReload={() => {
          setConflict(null);
          props.onReload();
        }}
        onOverwrite={() => void onOverwrite()}
      />
    </div>
  );
}
