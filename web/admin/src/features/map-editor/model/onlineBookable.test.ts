import { describe, expect, it } from 'vitest';
import { spotFromApi, spotToInput } from './document';

const apiSpot = {
  id: 's1',
  map_id: 'm1',
  zone_id: 'z1',
  label: 'A1',
  row_label: 'A',
  number: 1,
  kind: 'umbrella' as const,
  x: 10,
  y: 10,
  rotation: 0,
  shape: { type: 'circle' as const, w: 30, h: 30 },
  capacity: {},
  max_extra: {},
  attributes: [],
  active: true,
};

describe('online_bookable in the map editor (spec 30.11)', () => {
  it('defaults to sellable online and is always sent explicitly', () => {
    const draft = spotFromApi(apiSpot);
    expect(draft.online_bookable).toBe(true);
    expect(spotToInput(draft).online_bookable).toBe(true);
  });

  it('keeps a spot excluded from online sale', () => {
    const draft = spotFromApi({ ...apiSpot, online_bookable: false });
    expect(draft.online_bookable).toBe(false);
    expect(spotToInput(draft).online_bookable).toBe(false);
    expect(spotToInput({ ...draft, online_bookable: true }).online_bookable).toBe(true);
  });
});
