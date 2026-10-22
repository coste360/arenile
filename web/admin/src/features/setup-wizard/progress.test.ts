import { describe, expect, it } from 'vitest';
import {
  STEPS,
  initialProgress,
  loadProgress,
  saveProgress,
  stepsHandled,
  storageKey,
  wizardReducer,
  type WizardAction,
  type WizardProgress,
} from './progress';

const run = (p: WizardProgress, ...actions: WizardAction[]) => actions.reduce(wizardReducer, p);

function memoryStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => void data.delete(k),
    setItem: (k, v) => void data.set(k, v),
  };
}

const emptyStatus = {
  has_season: false,
  has_rate_periods: false,
  has_map: false,
  spot_count: 0,
  has_catalog: false,
  has_prices: false,
  booking_enabled: false,
};

describe('wizard navigation', () => {
  it('has the six steps of spec 15.2', () => {
    expect(STEPS).toEqual(['details', 'template', 'rows', 'pricelist', 'publicPage', 'payments']);
  });

  it('completes, skips, goes back and finishes', () => {
    let p = initialProgress(emptyStatus);
    expect(p.current).toBe(0);
    p = run(p, { type: 'complete' }, { type: 'skip' });
    expect(p.current).toBe(2);
    expect(p.states).toMatchObject({ details: 'done', template: 'skipped', rows: 'pending' });
    p = run(p, { type: 'back' });
    expect(p.current).toBe(1);
    p = run(p, { type: 'setTemplate', template: 'classic_rows' }, { type: 'complete' });
    expect(p.states.template).toBe('done');
    expect(p.template).toBe('classic_rows');
    p = run(p, { type: 'skip' }, { type: 'skip' }, { type: 'skip' }, { type: 'complete' });
    expect(p.finished).toBe(true);
    expect(p.current).toBe(5);
    expect(stepsHandled(p)).toBe(6);
  });

  it('every step is skippable and a done step stays done when skipped again', () => {
    let p = initialProgress();
    for (let i = 0; i < STEPS.length; i++) p = run(p, { type: 'skip' });
    expect(p.finished).toBe(true);
    expect(Object.values(p.states).every((s) => s === 'skipped')).toBe(true);
    p = run(
      p,
      { type: 'goto', step: 0 },
      { type: 'complete' },
      { type: 'goto', step: 0 },
      { type: 'skip' },
    );
    expect(p.states.details).toBe('done');
    expect(p.finished).toBe(false);
  });

  it('back never goes below the first step; goto ignores invalid steps', () => {
    const p = initialProgress();
    expect(run(p, { type: 'back' }).current).toBe(0);
    expect(run(p, { type: 'goto', step: 9 })).toBe(p);
  });

  it('keeps the ids created by apply-template', () => {
    const p = run(initialProgress(), { type: 'applied', seasonId: 's1', mapId: 'm1' });
    expect(p).toMatchObject({ seasonId: 's1', mapId: 'm1' });
    expect(run(p, { type: 'applied' })).toMatchObject({ seasonId: 's1', mapId: 'm1' });
  });
});

describe('resume', () => {
  it('derives the first pending step from the server status', () => {
    const p = initialProgress({ ...emptyStatus, has_season: true, has_map: true, spot_count: 120 });
    expect(p.states).toMatchObject({
      details: 'done',
      template: 'done',
      rows: 'done',
      pricelist: 'pending',
    });
    expect(p.current).toBe(3);
    const all = initialProgress({
      ...emptyStatus,
      has_season: true,
      has_prices: true,
      booking_enabled: true,
    });
    expect(all.current).toBe(5);
  });

  it('persists and reloads per establishment', () => {
    const storage = memoryStorage();
    const p = run(
      initialProgress(),
      { type: 'complete' },
      { type: 'setTemplate', template: 'empty' },
    );
    saveProgress(storage, 'e1', p);
    expect(loadProgress(storage, 'e1')).toEqual(p);
    expect(loadProgress(storage, 'e2')).toBeNull();
  });

  it('ignores corrupted or foreign data', () => {
    const storage = memoryStorage();
    storage.setItem(storageKey('e1'), '{not json');
    expect(loadProgress(storage, 'e1')).toBeNull();
    storage.setItem(
      storageKey('e1'),
      JSON.stringify({ v: 1, current: 42, states: {}, finished: false }),
    );
    expect(loadProgress(storage, 'e1')).toBeNull();
    expect(loadProgress(undefined, 'e1')).toBeNull();
  });

  it('survives a storage that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
      setItem: () => {
        throw new Error('quota');
      },
    } as unknown as Storage;
    expect(loadProgress(broken, 'e1')).toBeNull();
    expect(() => saveProgress(broken, 'e1', initialProgress())).not.toThrow();
  });
});
