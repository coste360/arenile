import type {
  ApiDecoration,
  ApiDecorationInput,
  ApiMapDocument,
  ApiMapSaveRequest,
  ApiSpot,
  ApiSpotInput,
  DraftDecoration,
  DraftDocument,
  DraftSpot,
  I18nText,
} from './types';

let seq = 0;

/** Client key for an element created in this session (never sent to the server). */
export function newKey(): string {
  seq += 1;
  return `new:${Date.now().toString(36)}:${seq}`;
}

export function isNewKey(key: string): boolean {
  return key.startsWith('new:');
}

export function spotFromApi(s: ApiSpot): DraftSpot {
  return {
    key: s.id,
    id: s.id,
    zone_id: s.zone_id,
    label: s.label,
    row_label: s.row_label ?? '',
    number: s.number ?? null,
    kind: s.kind,
    asset: s.asset ?? '',
    x: s.x,
    y: s.y,
    rotation: s.rotation ?? 0,
    shape: { ...s.shape },
    capacity: { ...(s.capacity ?? {}) },
    max_extra: { ...(s.max_extra ?? {}) },
    price_category_override: s.price_category_override ?? '',
    attributes: [...(s.attributes ?? [])],
    active: s.active,
    online_bookable: s.online_bookable ?? true,
  };
}

/** Spot returned by the row generator (no id yet) as a new draft spot. */
export function spotFromInput(s: ApiSpotInput): DraftSpot {
  return {
    key: newKey(),
    zone_id: s.zone_id,
    label: s.label,
    row_label: s.row_label ?? '',
    number: s.number ?? null,
    kind: s.kind,
    asset: s.asset ?? '',
    x: s.x,
    y: s.y,
    rotation: s.rotation ?? 0,
    shape: { ...s.shape },
    capacity: { ...(s.capacity ?? {}) },
    max_extra: { ...(s.max_extra ?? {}) },
    price_category_override: s.price_category_override ?? '',
    attributes: [...(s.attributes ?? [])],
    active: s.active ?? true,
  };
}

export function decorationFromApi(d: ApiDecoration): DraftDecoration {
  return {
    key: d.id,
    id: d.id,
    kind: d.kind,
    asset: d.asset ?? '',
    variant: d.variant ?? '',
    x: d.x,
    y: d.y,
    rotation: d.rotation ?? 0,
    shape: { ...d.shape },
    label: { ...(d.label ?? {}) },
  };
}

export function draftFromApi(doc: ApiMapDocument): DraftDocument {
  const m = doc.map;
  return {
    map: {
      name: { ...m.name },
      width: m.width,
      height: m.height,
      grid_size: m.grid_size,
      sea_side: m.sea_side,
      sort_order: m.sort_order,
      ...(m.background ? { background: m.background } : {}),
    },
    spots: doc.spots.map(spotFromApi),
    decorations: doc.decorations.map(decorationFromApi),
  };
}

function cleanText(t: I18nText): I18nText | undefined {
  const out: I18nText = {};
  for (const [k, v] of Object.entries(t)) if (v.trim()) out[k] = v.trim();
  return Object.keys(out).length ? out : undefined;
}

function nonEmpty<T extends object>(o: T): T | undefined {
  return Object.keys(o).length ? o : undefined;
}

export function spotToInput(s: DraftSpot): ApiSpotInput {
  const out: ApiSpotInput = {
    zone_id: s.zone_id,
    label: s.label,
    kind: s.kind,
    x: s.x,
    y: s.y,
    rotation: s.rotation,
    shape: s.shape,
    active: s.active,
  };
  if (s.id) out.id = s.id;
  if (s.asset) out.asset = s.asset;
  if (s.row_label) out.row_label = s.row_label;
  if (s.number != null) out.number = s.number;
  const capacity = nonEmpty(s.capacity);
  if (capacity) out.capacity = capacity;
  const maxExtra = nonEmpty(s.max_extra);
  if (maxExtra) out.max_extra = maxExtra;
  if (s.price_category_override) out.price_category_override = s.price_category_override;
  // Nullable lists: absent means "none", never an empty list.
  if (s.attributes.length) out.attributes = s.attributes;
  // Spec 30.11: always explicit, so switching it back on is saved too.
  out.online_bookable = s.online_bookable !== false;
  return out;
}

export function decorationToInput(d: DraftDecoration): ApiDecorationInput {
  const out: ApiDecorationInput = {
    kind: d.kind,
    x: d.x,
    y: d.y,
    rotation: d.rotation,
    shape: d.shape,
  };
  if (d.id) out.id = d.id;
  if (d.asset) out.asset = d.asset;
  if (d.variant) out.variant = d.variant;
  const label = cleanText(d.label);
  if (label) out.label = label;
  return out;
}

/**
 * Full-replacement save body (PUT /maps/{id}). `knownIds`, when given, drops
 * ids the server no longer has, so those elements are re-created instead of
 * being rejected (used by "overwrite" after a version conflict).
 */
export function toSaveRequest(
  draft: DraftDocument,
  knownIds?: { spots: ReadonlySet<string>; decorations: ReadonlySet<string> },
): ApiMapSaveRequest {
  const m = draft.map;
  return {
    map: {
      name: cleanText(m.name) ?? m.name,
      width: m.width,
      height: m.height,
      grid_size: m.grid_size,
      sea_side: m.sea_side,
      sort_order: m.sort_order,
      ...(m.background ? { background: m.background } : {}),
    },
    spots: draft.spots.map((s) => {
      const input = spotToInput(s);
      if (knownIds && input.id && !knownIds.spots.has(input.id)) delete input.id;
      return input;
    }),
    decorations: draft.decorations.map((d) => {
      const input = decorationToInput(d);
      if (knownIds && input.id && !knownIds.decorations.has(input.id)) delete input.id;
      return input;
    }),
  };
}

export interface DraftIssue {
  type: 'missing_zone' | 'duplicate_label' | 'empty_label';
  keys: string[];
  /** Label involved, for duplicate labels. */
  label?: string;
}

/** Client-side checks run before saving; the server validates again. */
export function validateDraft(draft: DraftDocument): DraftIssue[] {
  const issues: DraftIssue[] = [];
  const missingZone = draft.spots.filter((s) => !s.zone_id).map((s) => s.key);
  if (missingZone.length) issues.push({ type: 'missing_zone', keys: missingZone });
  const empty = draft.spots.filter((s) => !s.label.trim()).map((s) => s.key);
  if (empty.length) issues.push({ type: 'empty_label', keys: empty });
  const byLabel = new Map<string, string[]>();
  for (const s of draft.spots) {
    const l = s.label.trim().toLowerCase();
    if (!l) continue;
    byLabel.set(l, [...(byLabel.get(l) ?? []), s.key]);
  }
  for (const [, keys] of byLabel) {
    if (keys.length > 1) {
      const first = draft.spots.find((s) => s.key === keys[0]);
      issues.push({ type: 'duplicate_label', keys, label: first?.label });
    }
  }
  return issues;
}

/** Localised text with fallback: requested locale, default locale, any. */
export function localized(
  text: I18nText | null | undefined,
  locale: string,
  fallbackLocale = 'it',
): string {
  if (!text) return '';
  const base = locale.split('-')[0] ?? locale;
  return text[locale] ?? text[base] ?? text[fallbackLocale] ?? Object.values(text)[0] ?? '';
}
