import { describe, expect, it } from 'vitest';
import { generatorDefaults, mapSettingsDefaults } from './editor.schema';

describe('map editor defaults', () => {
  it('new maps have the sea at the top (spec 11)', () => {
    expect(mapSettingsDefaults(undefined, ['it']).sea_side).toBe('top');
  });

  it('the row generator starts row A next to the sea', () => {
    const map = { width: 1200, height: 800, grid_size: 10 };
    expect(generatorDefaults({ ...map, sea_side: 'top' }, 'z')).toMatchObject({
      origin_x: '60',
      origin_y: '60',
    });
    expect(generatorDefaults({ ...map, sea_side: 'bottom' }, 'z')).toMatchObject({
      origin_y: '740',
    });
    expect(generatorDefaults({ ...map, sea_side: 'right' }, 'z')).toMatchObject({
      origin_x: '1140',
    });
  });
});
