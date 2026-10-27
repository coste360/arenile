import {
  DECORATION_KIND_ICONS,
  KIT_BY_ID,
  SPOT_KIND_ICONS,
  decorationAsset,
  isTileAsset,
  spotAsset,
  type AlignMode,
} from '@arenile/shared/map';
import {
  ButtonGroup,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
  Field,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui';
import {
  AlignCenterHorizontalIcon,
  AlignCenterVerticalIcon,
  AlignEndHorizontalIcon,
  AlignEndVerticalIcon,
  AlignHorizontalDistributeCenterIcon,
  AlignStartHorizontalIcon,
  AlignStartVerticalIcon,
  AlignVerticalDistributeCenterIcon,
  CopyIcon,
  ListOrderedIcon,
  MousePointerClickIcon,
  RotateCcwIcon,
  RotateCwIcon,
  Trash2Icon,
  type LucideIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { ZoneOption } from './dialogs';
import { IllustrationField, VariantField } from './IllustrationField';
import { decorationKindPatch, spotKindPatch } from './model/editor';
import { ToolButton } from './ui';
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

const ALIGN_ICONS: Record<AlignMode, LucideIcon> = {
  left: AlignStartVerticalIcon,
  'center-x': AlignCenterVerticalIcon,
  right: AlignEndVerticalIcon,
  top: AlignStartHorizontalIcon,
  'center-y': AlignCenterHorizontalIcon,
  bottom: AlignEndHorizontalIcon,
};

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
    return <FieldDescription>{t('panel.noComponents')}</FieldDescription>;
  }
  return (
    <div className="overflow-hidden rounded-md border">
      <Table>
        <caption className="sr-only">{t('panel.quantities')}</caption>
        <TableHeader className="bg-muted/50">
          <TableRow className="hover:bg-transparent">
            <TableHead className="h-8 text-xs">{t('panel.component')}</TableHead>
            <TableHead className="h-8 w-20 text-xs">{t('fields.capacity')}</TableHead>
            <TableHead className="h-8 w-20 text-xs">{t('fields.max_extra')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {codes.map((c) => (
            <TableRow key={c.code} className="hover:bg-transparent">
              <TableHead scope="row" className="h-auto py-1 font-normal text-foreground">
                {c.name}
              </TableHead>
              {(['capacity', 'max_extra'] as const).map((field) => (
                <TableCell key={field} className="py-1">
                  <CommitInput
                    id={`spot-${field}-${c.code}`}
                    ariaLabel={`${t(`fields.${field}`)}: ${c.name}`}
                    inputMode="numeric"
                    className="h-8"
                    value={spot[field][c.code] != null ? String(spot[field][c.code]) : ''}
                    disabled={readOnly}
                    onCommit={(v) => {
                      const n = quantity(v);
                      if (n !== undefined)
                        onPatch({ [field]: withQuantity(spot[field], c.code, n) });
                    }}
                  />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
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
  className,
}: {
  id: string;
  value: string;
  onCommit: (v: string) => void;
  disabled?: boolean;
  inputMode?: 'numeric' | 'decimal';
  ariaLabel?: string;
  className?: string;
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
      className={className ?? 'h-8'}
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
    <Field className="gap-1.5">
      <FieldLabel htmlFor={id} className="text-xs text-muted-foreground">
        {label}
      </FieldLabel>
      {children}
    </Field>
  );
}

function ChoiceSelect({
  id,
  value,
  placeholder,
  disabled,
  onChange,
  options,
}: {
  id: string;
  value: string;
  placeholder?: string;
  disabled?: boolean;
  onChange: (v: string) => void;
  options: { value: string; label: ReactNode; disabled?: boolean }[];
}) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger id={id} size="sm" className="w-full">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value} disabled={o.disabled}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

const num = (v: string) => {
  const n = Number(v.replace(',', '.'));
  return Number.isFinite(n) ? n : null;
};

function withIcon(Icon: LucideIcon, label: string) {
  return (
    <>
      <Icon aria-hidden="true" />
      {label}
    </>
  );
}

/** Properties of the selected spot(s) or decoration, and bulk tools. */
export function SelectionPanel(p: SelectionPanelProps) {
  const { t } = useTranslation('mapEditor');
  const { selected, readOnly } = p;
  // The kind an illustration pick just changed the selection to, if any:
  // the kind is semantic (packs, assignment), so the change is announced.
  const [kindNotice, setKindNotice] = useState<SpotKind | null>(null);
  if (selected.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 px-2 py-8 text-center">
        <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <MousePointerClickIcon aria-hidden="true" className="size-5" />
        </span>
        <p className="text-sm text-muted-foreground">{t('panel.empty')}</p>
      </div>
    );
  }
  const spots = selected.filter(isSpot);
  const single = selected.length === 1 ? selected[0] : undefined;
  const sameZone = spots.every((s) => s.zone_id === spots[0]?.zone_id);
  const sameKind = spots.every((s) => s.kind === spots[0]?.kind);
  const sameAsset = spots.every((s) => s.asset === spots[0]?.asset && s.kind === spots[0]?.kind);

  return (
    <FieldGroup className="gap-5">
      <p className="text-sm font-medium" aria-live="polite">
        {t('panel.selected', { count: selected.length })}
      </p>
      {single && isSpot(single) && (
        <FieldSet className="gap-3">
          <FieldLegend variant="label">{t('panel.identity')}</FieldLegend>
          <Row id="spot-label" label={t('fields.label')}>
            <CommitInput
              id="spot-label"
              value={single.label}
              disabled={readOnly}
              onCommit={(label) => p.onSpotPatch({ label: label.trim() })}
            />
          </Row>
          <div className="grid grid-cols-2 gap-3">
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
          </div>
          <Row id="spot-category" label={t('fields.price_category_override')}>
            <CommitInput
              id="spot-category"
              value={single.price_category_override}
              disabled={readOnly}
              onCommit={(v) => p.onSpotPatch({ price_category_override: v.trim().toLowerCase() })}
            />
          </Row>
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
            <FieldDescription className="text-xs">{t('panel.attributesHint')}</FieldDescription>
          </Row>
          <FieldSeparator />
          <FieldLegend variant="label">{t('panel.geometry')}</FieldLegend>
          <div className="grid grid-cols-3 gap-3">
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
          </div>
        </FieldSet>
      )}
      {spots.length > 0 && (
        <FieldSet className="gap-3">
          {single && <FieldSeparator />}
          <FieldLegend variant="label">{t('panel.booking')}</FieldLegend>
          <Row id="spot-zone" label={t('fields.zone')}>
            <ChoiceSelect
              id="spot-zone"
              disabled={readOnly}
              value={sameZone ? (spots[0]?.zone_id ?? '') : ''}
              placeholder={t('panel.mixed')}
              onChange={(v) => v && p.onSpotPatch({ zone_id: v })}
              options={p.zones.map((z) => ({ value: z.id, label: z.name, disabled: z.disabled }))}
            />
          </Row>
          <Row id="spot-kind" label={t('fields.kind')}>
            <ChoiceSelect
              id="spot-kind"
              disabled={readOnly}
              value={sameKind ? (spots[0]?.kind ?? '') : ''}
              placeholder={t('panel.mixed')}
              onChange={(v) => {
                if (!v) return;
                setKindNotice(null);
                // An explicit illustration of another kind is dropped (bulk: when mixed).
                p.onSpotPatch(
                  spotKindPatch(v as SpotKind, sameAsset ? (spots[0]?.asset ?? '') : ''),
                );
              }}
              options={SPOT_KINDS.map((k) => ({
                value: k,
                label: withIcon(SPOT_KIND_ICONS[k], t(`kinds.${k}`)),
              }))}
            />
          </Row>
          <Row id="spot-asset" label={t('fields.illustration')}>
            <IllustrationField
              id="spot-asset"
              family="spots"
              disabled={readOnly}
              value={sameAsset ? (spots[0]?.asset ?? '') : undefined}
              effective={sameAsset && spots[0] ? spotAsset(spots[0]) : null}
              onPick={(a) => {
                const kind = (a.spotKind ?? 'custom') as SpotKind;
                setKindNotice(spots.some((s) => s.kind !== kind) ? kind : null);
                p.onSpotPatch({ asset: a.id, kind });
              }}
              onReset={() => p.onSpotPatch({ asset: '' })}
            />
            <FieldDescription className="text-xs">{t('panel.illustrationHint')}</FieldDescription>
            {kindNotice && (
              <FieldDescription role="status" className="text-xs text-foreground">
                {t('panel.kindChanged', { kind: t(`kinds.${kindNotice}`) })}
              </FieldDescription>
            )}
          </Row>
          <Field orientation="horizontal" className="items-start">
            <Switch
              id="spot-active"
              disabled={readOnly}
              checked={spots.every((s) => s.active)}
              onCheckedChange={(v) => p.onSpotPatch({ active: v })}
            />
            <div className="space-y-1">
              <FieldLabel htmlFor="spot-active">{t('fields.active')}</FieldLabel>
              <FieldDescription className="text-xs">{t('panel.deactivateHint')}</FieldDescription>
            </div>
          </Field>
          <Field orientation="horizontal" className="items-start">
            <Switch
              id="spot-online"
              disabled={readOnly}
              checked={spots.every((s) => s.online_bookable !== false)}
              onCheckedChange={(v) => p.onSpotPatch({ online_bookable: v })}
            />
            <div className="space-y-1">
              <FieldLabel htmlFor="spot-online">{t('fields.onlineBookable')}</FieldLabel>
              <FieldDescription className="text-xs">
                {t('panel.onlineBookableHint')}
              </FieldDescription>
            </div>
          </Field>
          {single && isSpot(single) && (
            <QuantitiesEditor
              spot={single}
              components={p.components}
              readOnly={readOnly}
              onPatch={p.onSpotPatch}
            />
          )}
        </FieldSet>
      )}
      {single && !isSpot(single) && (
        <FieldSet className="gap-3">
          <Row id="deco-kind" label={t('fields.decorationKind')}>
            <ChoiceSelect
              id="deco-kind"
              disabled={readOnly}
              value={single.kind}
              onChange={(v) =>
                p.onDecorationPatch(
                  single.key,
                  decorationKindPatch(v as DecorationKind, single.asset, single.variant),
                )
              }
              options={DECORATION_KINDS.map((k) => ({
                value: k,
                label: withIcon(DECORATION_KIND_ICONS[k], t(`decorations.${k}`)),
              }))}
            />
          </Row>
          {single.kind !== 'text' && (
            <Row id="deco-asset" label={t('fields.illustration')}>
              <IllustrationField
                id="deco-asset"
                family="decorations"
                disabled={readOnly}
                value={single.asset}
                effective={decorationAsset(single)}
                onPick={(a) =>
                  p.onDecorationPatch(single.key, {
                    asset: a.id,
                    kind: (a.decorationKind ?? 'custom') as DecorationKind,
                    variant: a.variants?.includes(single.variant) ? single.variant : '',
                  })
                }
                onReset={() => p.onDecorationPatch(single.key, { asset: '', variant: '' })}
              />
              {isTileAsset(decorationAsset(single)) && (
                <FieldDescription className="text-xs">{t('panel.tileHint')}</FieldDescription>
              )}
            </Row>
          )}
          {(() => {
            const variants = KIT_BY_ID.get(decorationAsset(single) ?? '')?.variants;
            return variants ? (
              <Row id="deco-variant" label={t('fields.flagColour')}>
                <VariantField
                  id="deco-variant"
                  variants={variants}
                  value={single.variant}
                  disabled={readOnly}
                  onChange={(variant) => p.onDecorationPatch(single.key, { variant })}
                />
              </Row>
            ) : null;
          })()}
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
          <div className="grid grid-cols-2 gap-3">
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
        </FieldSet>
      )}
      {!readOnly && (
        <FieldSet className="gap-3">
          <FieldSeparator />
          <FieldLegend variant="label">{t('panel.arrange')}</FieldLegend>
          <div className="flex flex-wrap gap-2">
            <ButtonGroup>
              <ToolButton
                variant="outline"
                icon={RotateCcwIcon}
                label={t('tools.rotateLeft')}
                onClick={() => p.onRotate(-15)}
              />
              <ToolButton
                variant="outline"
                icon={RotateCwIcon}
                label={t('tools.rotateRight')}
                onClick={() => p.onRotate(15)}
              />
            </ButtonGroup>
            <ToolButton
              variant="outline"
              icon={CopyIcon}
              label={t('tools.duplicate')}
              shortcut={['Mod', 'D']}
              onClick={p.onDuplicate}
            />
            {spots.length > 0 && (
              <ToolButton
                variant="outline"
                icon={ListOrderedIcon}
                label={t('tools.renumber')}
                text={t('tools.renumber')}
                onClick={p.onRenumber}
              />
            )}
          </div>
          {selected.length > 1 && (
            <ButtonGroup aria-label={t('tools.align')}>
              {(['left', 'center-x', 'right', 'top', 'center-y', 'bottom'] as const).map((m) => (
                <ToolButton
                  key={m}
                  variant="outline"
                  icon={ALIGN_ICONS[m]}
                  label={t(`tools.alignModes.${m}`)}
                  onClick={() => p.onAlign(m)}
                />
              ))}
            </ButtonGroup>
          )}
          {selected.length > 2 && (
            <ButtonGroup>
              <ToolButton
                variant="outline"
                icon={AlignHorizontalDistributeCenterIcon}
                label={t('tools.distributeX')}
                onClick={() => p.onDistribute('x')}
              />
              <ToolButton
                variant="outline"
                icon={AlignVerticalDistributeCenterIcon}
                label={t('tools.distributeY')}
                onClick={() => p.onDistribute('y')}
              />
            </ButtonGroup>
          )}
          <ToolButton
            variant="destructive"
            icon={Trash2Icon}
            label={t('tools.delete', { count: selected.length })}
            text={t('tools.delete', { count: selected.length })}
            className="w-fit"
            onClick={p.onDelete}
          />
        </FieldSet>
      )}
    </FieldGroup>
  );
}
