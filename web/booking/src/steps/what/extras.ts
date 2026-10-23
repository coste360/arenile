import type { Schemas } from '@arenile/shared/api';

/** Extras of a line compatible with its pack (attach_to = line). */
export function lineExtras(
  extras: readonly Schemas['PublicExtra'][],
  packId: string | null,
): Schemas['PublicExtra'][] {
  if (!packId) return [];
  return extras.filter(
    (e) =>
      e.attach_to === 'line' &&
      (!e.allowed_pack_ids ||
        e.allowed_pack_ids.length === 0 ||
        e.allowed_pack_ids.includes(packId)),
  );
}
