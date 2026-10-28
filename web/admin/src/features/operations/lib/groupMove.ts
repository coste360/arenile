import type { MapSpot } from '@arenile/shared/map';

export interface GroupMoveTarget {
  /** Spot the line leaves. */
  fromSpotId: string;
  /** Spot it moves to; undefined when no spot sits at that place. */
  toSpotId?: string;
}

/**
 * Destinations of a group move that keep the relative arrangement (spec
 * 30.7): the anchor spot goes to `anchorTarget` and every other spot is
 * shifted by the same offset on the drawing; each one lands on the spot
 * closest to its shifted centre, within 60% of the spot size. Spots are
 * never reused: a shift that lands two spots on the same one leaves the
 * second unresolved. Pure; the server checks availability again.
 */
export function groupMoveTargets(
  spots: readonly MapSpot[],
  sourceIds: readonly string[],
  anchorId: string,
  anchorTarget: string,
): GroupMoveTarget[] {
  const byId = new Map(spots.map((s) => [s.id, s]));
  const anchor = byId.get(anchorId);
  const target = byId.get(anchorTarget);
  if (!anchor || !target) return sourceIds.map((id) => ({ fromSpotId: id }));
  const dx = target.x - anchor.x;
  const dy = target.y - anchor.y;
  // The anchor's destination is taken first, whatever the order of the sources.
  const used = new Set<string>([target.id]);
  return sourceIds.map((id) => {
    const from = byId.get(id);
    if (!from) return { fromSpotId: id };
    if (id === anchorId) return { fromSpotId: id, toSpotId: target.id };
    const x = from.x + dx;
    const y = from.y + dy;
    const tol = Math.max(from.shape.w, from.shape.h) * 0.6;
    let best: MapSpot | undefined;
    let bestDist = Infinity;
    for (const s of spots) {
      if (s.active === false || used.has(s.id)) continue;
      const d = Math.hypot(s.x - x, s.y - y);
      if (d <= tol && d < bestDist) {
        best = s;
        bestDist = d;
      }
    }
    if (!best) return { fromSpotId: id };
    used.add(best.id);
    return { fromSpotId: id, toSpotId: best.id };
  });
}
