import { Button, Input, Label } from '@arenile/shared/ui';
import type { AlignMode } from '@arenile/shared/map';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '../configuration/components/fields';
import type { ZoneOption } from './dialogs';
import {
  DECORATION_KINDS,
  SPOT_KINDS,
  isSpot,
  type DecorationKind,
  type DraftDecoration,
  type DraftElement,
  type DraftSpot,
  type SpotKind,
} from './model/types';

export interface ComponentOption {
  code: string;
  name: string;
}

export interface SelectionPanelProps {
  selected: DraftElement[];
  zones: ZoneOption[];
  /** Catalog components, for capacity and max extra quantities. */
  components: ComponentOption[];
  defaultLocale: string;
  readOnly?: boolean;
  onSpotPatch: (patch: Partial<Omit<DraftSpot, 'key' | 'id'>>) => void;
  onDecorationPatch: (key: string, patch: Partial<Omit<DraftDecoration, 'key' | 'id'>>) => void;
  onAlign: (mode: AlignMode) => void;
  onDistribute: (axis: 'x' | 'y') => void;
  onRotate: (deg: number) => void;
  onDuplicate: () => void;
  /** Opens the row renumbering dialog for the selected spots. */
  onRenumber: () => void;
  onDelete: () => void;
}

/** Parses a quantity field: empty = not set, otherwise a non-negative integer. */
function quantity(v: string): number | null | undefined {
  const s = v.trim();
  if (s === '') return null;
  const n = Number(s);
  return Number.isInteger(n) && n >= 0 ? n : undefined;
}

function withQuantity(q: Record<string, number>, code: string, n: number | null) {
  const out = { ...q };
  if (n === null || n === 0) delete out[code];
  else out[code] = n;
  return out;
}

/** Capacity and maximum extra quantities per catalog component (single spot). */
function QuantitiesEditor({
  spot,
  components,
  readOnly,
  onPatch,
}: {
  spot: DraftSpot;
  components: ComponentOption[];
  readOnly?: boolean;
  onPatch: SelectionPanelProps['onSpotPatch'];
}) {
  const { t } = useTranslation('mapEditor');
  // Codes on the spot but missing from the catalog are still shown (and editable).
  const codes = [
    ...components,
    ...Object.keys({ ...spot.capacity, ...spot.max_extra })
      .filter((c) => !components.some((k) => k.code === c))
      .map((code) => ({ code, name: code })),
  ];
  if (codes.length === 0) {
    return <p className="text-xs text-slate-600">{t('panel.noComponents')}</p>;
  }
  return (
    <table className="w-full text-sm">
      <caption className="sr-only">{t('panel.quantities')}</caption>
      <thead>
        <tr className="text-left text-xs text-slate-600">
          <th scope="col" className="font-medium">
            {t('panel.component')}
          </th>
          <th scope="col" className="w-16 font-medium">
            {t('fields.capacity')}
          </th>
          <th scope="col" className="w-16 font-medium">
            {t('fields.max_extra')}
          </th>
        </tr>
      </thead>
      <tbody>
        {codes.map((c) => (
          <tr key={c.code}>
            <th scope="row" className="pr-2 text-left font-normal">
              {c.name}
            </th>
            {(['capacity', 'max_extra'] as const).map((field) => (
              <td key={field} className="py-0.5">
                <CommitInput
                  id={`spot-${field}-${c.code}`}
                  ariaLabel={`${t(`fields.${field}`)}: ${c.name}`}
                  inputMode="numeric"
                  value={spot[field][c.code] != null ? String(spot[field][c.code]) : ''}
                  disabled={readOnly}
                  onCommit={(v) => {
                    const n = quantity(v);
                    if (n !== undefined) onPatch({ [field]: withQuantity(spot[field], c.code, n) });
                  }}
                />
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** Text input that commits on blur or Enter (one undo step per edit, not per key). */
function CommitInput({
  id,
  value,
  onCommit,
  disabled,
  inputMode,
  ariaLabel,
}: {
  id: string;
  value: string;
  onCommit: (v: string) => void;
  disabled?: boolean;
  inputMode?: 'numeric' | 'decimal';
  ariaLabel?: string;
}) {
  // null = not editing: show the current value (follows undo/redo and selection changes).
  const [draft, setDraft] = useState<string | null>(null);
  const text = draft ?? value;
  const commit = () => {
    if (draft !== null && draft !== value) onCommit(draft);
    setDraft(null);
  };
  return (
    <Input
      id={id}
      value={text}
      disabled={disabled}
      inputMode={inputMode}
      aria-label={ariaLabel}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          commit();
        }
      }}
    />
  );
}

function Row({ id, label, children }: { id: string; label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-xs text-slate-600">
        {label}
      </Label>
      {children}
    </div>
  );
}

const num = (v: string) => {
  const n = Number(v.replace(',', '.'));
  return Number.isFinite(n) ? n : null;
};

/** Properties of the selected spot(s) or decoration, and bulk tools. */
export function SelectionPanel(p: SelectionPanelProps) {
  const { t } = useTranslation('mapEditor');
  const { selected, readOnly } = p;
  if (selected.length === 0) {
    return <p className="text-sm text-slate-600">{t('panel.empty')}</p>;
  }
  const spots = selected.filter(isSpot);
  const single = selected.length === 1 ? selected[0] : undefined;

  return (
    <div className="space-y-4">
      <p className="text-sm font-medium" aria-live="polite">
        {t('panel.selected', { count: selected.length })}
      </p>
      {single && isSpot(single) && (
        <div className="space-y-3">
          <Row id="spot-label" label={t('fields.label')}>
            <CommitInput
              id="spot-label"
              value={single.label}
              disabled={readOnly}
              onCommit={(label) => p.onSpotPatch({ label: label.trim() })}
            />
          </Row>
          <div className="grid grid-cols-2 gap-2">
            <Row id="spot-row" label={t('fields.row_label')}>
              <CommitInput
                id="spot-row"
                value={single.row_label}
                disabled={readOnly}
                onCommit={(row_label) => p.onSpotPatch({ row_label: row_label.trim() })}
              />
            </Row>
            <Row id="spot-number" label={t('fields.number')}>
              <CommitInput
                id="spot-number"
                inputMode="numeric"
                value={single.number == null ? '' : String(single.number)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = v.trim() === '' ? null : num(v);
                  if (n === null || (Number.isInteger(n) && n >= 0)) p.onSpotPatch({ number: n });
                }}
              />
            </Row>
            <Row id="spot-x" label="X">
              <CommitInput
                id="spot-x"
                inputMode="decimal"
                value={String(single.x)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = num(v);
                  if (n !== null) p.onSpotPatch({ x: n });
                }}
              />
            </Row>
            <Row id="spot-y" label="Y">
              <CommitInput
                id="spot-y"
                inputMode="decimal"
                value={String(single.y)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = num(v);
                  if (n !== null) p.onSpotPatch({ y: n });
                }}
              />
            </Row>
            <Row id="spot-size" label={t('fields.size')}>
              <CommitInput
                id="spot-size"
                inputMode="decimal"
                value={String(single.shape.w)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = num(v);
                  if (n !== null && n > 0) {
                    p.onSpotPatch({
                      shape: {
                        ...single.shape,
                        w: n,
                        h: single.shape.type === 'circle' ? n : single.shape.h,
                      },
                    });
                  }
                }}
              />
            </Row>
            <Row id="spot-category" label={t('fields.price_category_override')}>
              <CommitInput
                id="spot-category"
                value={single.price_category_override}
                disabled={readOnly}
                onCommit={(v) => p.onSpotPatch({ price_category_override: v.trim().toLowerCase() })}
              />
            </Row>
          </div>
          <Row id="spot-attributes" label={t('fields.attributes')}>
            <CommitInput
              id="spot-attributes"
              value={single.attributes.join(', ')}
              disabled={readOnly}
              onCommit={(v) =>
                p.onSpotPatch({
                  attributes: [
                    ...new Set(
                      v
                        .split(',')
                        .map((a) => a.trim().toLowerCase())
                        .filter(Boolean),
                    ),
                  ],
                })
              }
            />
          </Row>
          <p className="text-xs text-slate-600">{t('panel.attributesHint')}</p>
          <QuantitiesEditor
            spot={single}
            components={p.components}
            readOnly={readOnly}
            onPatch={p.onSpotPatch}
          />
        </div>
      )}
      {spots.length > 0 && (
        <div className="space-y-3">
          <Row id="spot-zone" label={t('fields.zone')}>
            <Select
              id="spot-zone"
              disabled={readOnly}
              value={
                spots.every((s) => s.zone_id === spots[0]?.zone_id) ? (spots[0]?.zone_id ?? '') : ''
              }
              onChange={(e) => e.target.value && p.onSpotPatch({ zone_id: e.target.value })}
            >
              <option value="">{t('panel.mixed')}</option>
              {p.zones.map((z) => (
                <option key={z.id} value={z.id} disabled={z.disabled}>
                  {z.name}
                </option>
              ))}
            </Select>
          </Row>
          <Row id="spot-kind" label={t('fields.kind')}>
            <Select
              id="spot-kind"
              disabled={readOnly}
              value={spots.every((s) => s.kind === spots[0]?.kind) ? (spots[0]?.kind ?? '') : ''}
              onChange={(e) =>
                e.target.value && p.onSpotPatch({ kind: e.target.value as SpotKind })
              }
            >
              <option value="">{t('panel.mixed')}</option>
              {SPOT_KINDS.map((k) => (
                <option key={k} value={k}>
                  {t(`kinds.${k}`)}
                </option>
              ))}
            </Select>
          </Row>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4 accent-[var(--ar-primary)]"
              disabled={readOnly}
              checked={spots.every((s) => s.active)}
              onChange={(e) => p.onSpotPatch({ active: e.target.checked })}
            />
            {t('fields.active')}
          </label>
          <p className="text-xs text-slate-600">{t('panel.deactivateHint')}</p>
        </div>
      )}
      {single && !isSpot(single) && (
        <div className="space-y-3">
          <Row id="deco-kind" label={t('fields.decorationKind')}>
            <Select
              id="deco-kind"
              disabled={readOnly}
              value={single.kind}
              onChange={(e) =>
                p.onDecorationPatch(single.key, { kind: e.target.value as DecorationKind })
              }
            >
              {DECORATION_KINDS.map((k) => (
                <option key={k} value={k}>
                  {t(`decorations.${k}`)}
                </option>
              ))}
            </Select>
          </Row>
          <Row id="deco-label" label={t('fields.decorationLabel')}>
            <CommitInput
              id="deco-label"
              value={single.label[p.defaultLocale] ?? ''}
              disabled={readOnly}
              onCommit={(v) =>
                p.onDecorationPatch(single.key, {
                  label: { ...single.label, [p.defaultLocale]: v.trim() },
                })
              }
            />
          </Row>
          <div className="grid grid-cols-2 gap-2">
            <Row id="deco-w" label={t('fields.width')}>
              <CommitInput
                id="deco-w"
                inputMode="decimal"
                value={String(single.shape.w)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = num(v);
                  if (n !== null && n > 0) {
                    p.onDecorationPatch(single.key, { shape: { ...single.shape, w: n } });
                  }
                }}
              />
            </Row>
            <Row id="deco-h" label={t('fields.height')}>
              <CommitInput
                id="deco-h"
                inputMode="decimal"
                value={String(single.shape.h)}
                disabled={readOnly}
                onCommit={(v) => {
                  const n = num(v);
                  if (n !== null && n > 0) {
                    p.onDecorationPatch(single.key, { shape: { ...single.shape, h: n } });
                  }
                }}
              />
            </Row>
          </div>
        </div>
      )}
      {!readOnly && (
        <div className="space-y-2 border-t border-slate-200 pt-3">
          <div className="flex flex-wrap gap-1">
            <Button size="sm" variant="outline" onClick={() => p.onRotate(-15)}>
              {t('tools.rotateLeft')}
            </Button>
            <Button size="sm" variant="outline" onClick={() => p.onRotate(15)}>
              {t('tools.rotateRight')}
            </Button>
            <Button size="sm" variant="outline" onClick={p.onDuplicate}>
              {t('tools.duplicate')}
            </Button>
            {spots.length > 0 && (
              <Button size="sm" variant="outline" onClick={p.onRenumber}>
                {t('tools.renumber')}
              </Button>
            )}
          </div>
          {selected.length > 1 && (
            <div className="flex flex-wrap gap-1" role="group" aria-label={t('tools.align')}>
              {(['left', 'center-x', 'right', 'top', 'center-y', 'bottom'] as const).map((m) => (
                <Button key={m} size="sm" variant="outline" onClick={() => p.onAlign(m)}>
                  {t(`tools.alignModes.${m}`)}
                </Button>
              ))}
            </div>
          )}
          {selected.length > 2 && (
            <div className="flex flex-wrap gap-1">
              <Button size="sm" variant="outline" onClick={() => p.onDistribute('x')}>
                {t('tools.distributeX')}
              </Button>
              <Button size="sm" variant="outline" onClick={() => p.onDistribute('y')}>
                {t('tools.distributeY')}
              </Button>
            </div>
          )}
          <Button size="sm" variant="destructive" onClick={p.onDelete}>
            {t('tools.delete', { count: selected.length })}
          </Button>
        </div>
      )}
    </div>
  );
}
