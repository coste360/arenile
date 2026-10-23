import { ApiProblem } from '@arenile/shared/api';
import type { AlignMode, KitAsset, Point } from '@arenile/shared/map';
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  ButtonGroup,
  ScrollArea,
  Separator,
  Spinner,
  Skeleton,
} from '@arenile/shared/ui';
import { BackLink, FormDialog, PageHeader, QueryState } from '@arenile/shared/ui/patterns';
import {
  ArrowLeftIcon,
  CheckIcon,
  CircleAlertIcon,
  InfoIcon,
  KeyboardIcon,
  PanelRightIcon,
  Redo2Icon,
  Rows3Icon,
  SaveIcon,
  Settings2Icon,
  TriangleAlertIcon,
  Undo2Icon,
} from 'lucide-react';
import { Link, useParams } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  components as componentsResource,
  zones as zonesResource,
} from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { localize } from '../configuration/lib/i18nText';
import { problemMessageKey } from '@arenile/shared/api';
import { MapSaveProblem, mapDocumentKey, useMapDocument, useSaveMap } from './api';
import { ConflictDialog, type ConflictInfo } from './ConflictDialog';
import { MapSettingsForm, RenumberRowForm, RowGeneratorForm, type ZoneOption } from './dialogs';
import { MapCanvas } from './MapCanvas';
import { SeaSideDialog } from './SeaSideDialog';
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
  type DraftMapSettings,
  type MapReorientResult,
  type DecorationKind,
  type DraftDocument,
  type DraftSpot,
  type Selection,
  type SpotKind,
} from './model/types';
import { ZONE_PALETTE } from './model/zoneColors';
import { Palette } from './Palette';
import { SelectionPanel, type ComponentOption } from './SelectionPanel';
import { ReadOnlyNotice, ToolButton } from './ui';

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
  const qc = useQueryClient();
  // The offline copy restored at startup is invalidated until refetched: it
  // must never seed the draft (the editor keeps its draft and never refetches
  // behind the user's back), so a mount that finds the copy waits for the
  // first fetch; offline, the editor stays on its skeleton (configuration is
  // not edited offline, spec 21.2).
  const [startedFromCopy] = useState(
    () => qc.getQueryState(mapDocumentKey(ctx.establishmentId, mapId))?.isInvalidated === true,
  );
  const serverDoc = startedFromCopy && !docQuery.isFetchedAfterMount ? undefined : docQuery.data;
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

  const readOnly = !ctx.isLoading && !ctx.canConfigure;
  return (
    <section aria-labelledby="page-title" className="space-y-4">
      <PageHeader
        eyebrow={
          <BackLink>
            <Link to="/e/$establishmentId/maps" params={{ establishmentId: ctx.establishmentId }}>
              <ArrowLeftIcon aria-hidden="true" />
              {t('editor.backToList')}
            </Link>
          </BackLink>
        }
        title={
          serverDoc
            ? t('editor.titleNamed', {
                name: localize(serverDoc.map.name, ctx.uiLocale, ctx.defaultLocale),
              })
            : t('editor.title')
        }
        description={t('editor.description')}
      />
      {readOnly && <ReadOnlyNotice />}
      <QueryState
        skeleton={
          <>
            <Skeleton className="h-11 w-full" />
            <Skeleton className="h-[60vh] w-full" />
          </>
        }
        isLoading={(!serverDoc && !docQuery.error) || zoneList.isPending}
        error={docQuery.error ?? zoneList.error}
        onRetry={() => void docQuery.refetch()}
      />
      {serverDoc && zoneList.data && (
        <Editor
          key={`${serverDoc.map.id}:${generation}`}
          serverDoc={serverDoc}
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
    </section>
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
  return !!el.closest(
    'input, textarea, select, [contenteditable="true"], [role="dialog"], [role="alertdialog"], [role="listbox"]',
  );
}

function Editor(props: EditorProps) {
  const { serverDoc, establishmentId, mapId, readOnly, defaultLocale, uiLocale } = props;
  const { t } = useTranslation('mapEditor');
  const [panelOpen, setPanelOpen] = useState(true);
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
  // Settings whose sea side change waits for "flip the layout" or "only the sea".
  const [seaChange, setSeaChange] = useState<DraftMapSettings | null>(null);
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

  /**
   * The server flipped or rotated the saved layout (new version): it becomes
   * the saved draft; name and grid edited in the same dialog stay as
   * unsaved changes on top of it.
   */
  const onFlipped = (res: MapReorientResult, settings: DraftMapSettings) => {
    const next = draftFromApi(res.document);
    setHistory(initHistory(next));
    setSavedDoc(next);
    setVersion(res.document.map.version);
    setSelection(new Set());
    setConflict(null);
    setSaveState({ kind: 'saved' });
    const { name, grid_size } = settings;
    if (
      grid_size !== next.map.grid_size ||
      JSON.stringify(name) !== JSON.stringify(next.map.name)
    ) {
      setHistory((h) => pushHistory(h, updateMapSettings(next, { name, grid_size })));
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

  const addSpotOfAsset = (asset: KitAsset) => {
    if (!firstZone) return;
    const kind = (asset.spotKind ?? 'custom') as SpotKind;
    // New spots go in the zone of the selected spot, if any.
    const zone =
      selectedSpots.find((s) => zoneOptions.some((z) => z.id === s.zone_id && !z.disabled))
        ?.zone_id ?? firstZone;
    const s = newSpot(doc, zone, { kind, asset: asset.id, at: freePosition(doc, center()) });
    commit(addSpots(doc, [s]));
    setSelection(new Set([s.key]));
  };

  const addDecorationOfKind = (kind: DecorationKind, asset = '') => {
    const d = newDecoration(doc, kind, freePosition(doc, center()), asset);
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

  const usedZones = zoneOptions.filter((z) => doc.spots.some((s) => s.zone_id === z.id));

  const canvas = (
    <div className="space-y-2">
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
      {usedZones.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs font-medium text-muted-foreground">{t('panel.zones')}</span>
          {usedZones.map((z) => (
            <Badge key={z.id} variant="outline" className="gap-1.5 font-normal">
              <svg viewBox="0 0 10 10" aria-hidden="true" className="size-2.5">
                <circle cx={5} cy={5} r={5} fill={zoneColor(z.id)} />
              </svg>
              {z.name}
            </Badge>
          ))}
        </div>
      )}
    </div>
  );

  const panel = (
    <aside
      id="map-panel"
      aria-labelledby="map-panel-title"
      className="flex h-full flex-col overflow-hidden rounded-lg border bg-card"
    >
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 id="map-panel-title" className="text-sm font-semibold">
          {t('panel.title')}
        </h2>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-4">
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
        </div>
      </ScrollArea>
      <div className="border-t bg-muted/40 px-4 py-3">
        <p className="flex gap-2 text-xs text-muted-foreground">
          <KeyboardIcon aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
          <span>
            <span className="sr-only">{t('panel.shortcuts')}: </span>
            {t('panel.keyboardHint')}
          </span>
        </p>
      </div>
    </aside>
  );

  return (
    <div className="space-y-3">
      <div
        className="flex flex-wrap items-center gap-1.5 rounded-lg border bg-card p-1.5 shadow-xs"
        role="toolbar"
        aria-label={t('tools.label')}
      >
        <ButtonGroup>
          <ToolButton
            icon={Undo2Icon}
            label={t('tools.undo')}
            shortcut={['Mod', 'Z']}
            aria-keyshortcuts="Control+Z Meta+Z"
            onClick={() => setHistory(undo)}
            disabled={history.past.length === 0}
          />
          <ToolButton
            icon={Redo2Icon}
            label={t('tools.redo')}
            shortcut={['Mod', 'Shift', 'Z']}
            aria-keyshortcuts="Control+Shift+Z Meta+Shift+Z"
            onClick={() => setHistory(redo)}
            disabled={history.future.length === 0}
          />
        </ButtonGroup>
        {!readOnly && (
          <>
            <Separator orientation="vertical" className="mx-1 h-6!" />
            <Palette
              canAddSpots={!!firstZone}
              onAddSpot={addSpotOfAsset}
              onAddDecoration={(a) =>
                addDecorationOfKind((a.decorationKind ?? 'custom') as DecorationKind, a.id)
              }
              onAddText={() => addDecorationOfKind('text')}
            />
            <Separator orientation="vertical" className="mx-1 h-6!" />
            <ToolButton
              icon={Rows3Icon}
              label={t('generator.description')}
              text={t('tools.generator')}
              disabled={!firstZone}
              onClick={() => setDialog('generator')}
            />
            <ToolButton
              icon={Settings2Icon}
              label={t('tools.settings')}
              onClick={() => setDialog('settings')}
            />
          </>
        )}
        <div className="ml-auto flex items-center gap-2 pl-2">
          <ToolButton
            icon={PanelRightIcon}
            label={t('tools.togglePanel')}
            aria-pressed={panelOpen}
            aria-controls={panelOpen ? 'map-panel' : undefined}
            onClick={() => setPanelOpen((o) => !o)}
          />
          {dirty ? (
            <span aria-hidden="true" className="size-2 rounded-full bg-highlight" />
          ) : saveState.kind === 'saved' ? (
            <CheckIcon aria-hidden="true" className="size-4 text-success" />
          ) : null}
          <span className="text-sm text-muted-foreground tabular-nums" aria-live="polite">
            {t('editor.count', { count: doc.spots.length })}
            {dirty ? ` · ${t('editor.unsaved')}` : ''}
            {saveState.kind === 'saved' ? ` · ${t('editor.saved')}` : ''}
          </span>
          <ToolButton
            variant="default"
            icon={save.isPending ? Spinner : SaveIcon}
            label={t('tools.save')}
            text={
              save.isPending ? t('common.actions.saving', { ns: 'translation' }) : t('tools.save')
            }
            shortcut={['Mod', 'S']}
            aria-keyshortcuts="Control+S Meta+S"
            onClick={() => void onSave()}
            disabled={readOnly || !dirty || save.isPending || issues.length > 0}
          />
        </div>
      </div>
      {!firstZone && !readOnly && (
        <Alert role="note">
          <InfoIcon aria-hidden="true" />
          <AlertTitle>{t('editor.noZonesTitle')}</AlertTitle>
          <AlertDescription>{t('editor.noZones')}</AlertDescription>
        </Alert>
      )}
      {issues.length > 0 && (
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('editor.issuesTitle')}</AlertTitle>
          <AlertDescription>
            <ul className="list-inside list-disc">
              {issues.map((i) => (
                <li key={`${i.type}:${i.keys.join(',')}`}>
                  {t(`issues.${i.type}`, { count: i.keys.length, label: i.label ?? '' })}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}
      {saveState.kind === 'allocated' && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('editor.allocatedTitle')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{t('editor.allocated', { count: saveState.labels.length })}</p>
            <div className="flex flex-wrap gap-1">
              {saveState.labels.map((l) => (
                <Badge key={l} variant="outline" className="font-mono">
                  {l}
                </Badge>
              ))}
            </div>
            <p>{t('panel.deactivateHint')}</p>
          </AlertDescription>
        </Alert>
      )}
      {saveState.kind === 'error' && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('editor.saveErrorTitle')}</AlertTitle>
          <AlertDescription>
            <ul className="list-inside list-disc">
              {saveState.messages.map((m) => (
                <li key={m}>{m}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}
      <div
        className={
          panelOpen ? 'grid gap-3 lg:grid-cols-[minmax(0,1fr)_22rem] lg:items-start' : 'grid gap-3'
        }
      >
        {canvas}
        {panelOpen && <div className="lg:sticky lg:top-4 lg:h-[65vh]">{panel}</div>}
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
              setDialog(null);
              // A new sea side on a map with elements: offer to flip the layout.
              if (v.sea_side !== doc.map.sea_side && elements(doc).length > 0) {
                setSeaChange({ ...doc.map, ...v });
                return;
              }
              commit(updateMapSettings(doc, v));
            }}
          />
        )}
      </FormDialog>
      {seaChange && (
        <SeaSideDialog
          open
          onOpenChange={(o) => !o && setSeaChange(null)}
          establishmentId={establishmentId}
          mapId={mapId}
          version={version}
          from={doc.map.sea_side}
          to={seaChange.sea_side}
          block={
            dirty
              ? 'dirty'
              : seaChange.width !== doc.map.width || seaChange.height !== doc.map.height
                ? 'resized'
                : null
          }
          onOnlySea={() => commit(updateMapSettings(doc, seaChange))}
          onFlipped={(res) => onFlipped(res, seaChange)}
        />
      )}
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
