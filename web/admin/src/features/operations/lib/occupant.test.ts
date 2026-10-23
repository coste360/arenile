import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { opBooking, opSpot } from '../testing/fixtures';
import it_ from '../locales/it.json';
import en from '../locales/en.json';
import {
  labelModeKey,
  occupantCaptions,
  occupantFullName,
  occupantShortLabel,
  useLabelMode,
} from './occupant';

/** Minimal t over the feature's locale files. */
function tFor(dict: Record<string, unknown>) {
  return (key: string, opts?: Record<string, unknown>) => {
    const v = key.split('.').reduce<unknown>((o, k) => (o as Record<string, unknown>)?.[k], dict);
    return String(v ?? key).replace(/\{\{(\w+)\}\}/g, (_, k: string) => String(opts?.[k] ?? ''));
  };
}
const t = tFor(it_);

describe('occupant labels', () => {
  it('uses the surname from the server, the full display name for the accessible name', () => {
    const b = opBooking({ occupant_label: 'Rossi', customer_display_name: 'Rossi M.' });
    expect(occupantShortLabel(b, t)).toBe('Rossi');
    expect(occupantFullName(b, t)).toBe('Rossi M.');
  });

  it('falls back to the first name when there is no surname (server label)', () => {
    const b = opBooking({ occupant_label: 'Anna', customer_display_name: '' });
    expect(occupantShortLabel(b, t)).toBe('Anna');
    expect(occupantFullName(b, t)).toBe('Anna');
  });

  it('anonymised customers read "Anonimo" on the map and "Cliente anonimo" in full, never the stored name', () => {
    const b = opBooking({
      occupant_label: '',
      customer_display_name: 'Anonimizzato',
      customer_anonymized: true,
    });
    expect(occupantShortLabel(b, t)).toBe('Anonimo');
    expect(occupantFullName(b, t)).toBe('Cliente anonimo');
    expect(occupantFullName(b, tFor(en))).toBe('Anonymous customer');
  });

  it('missing data: no label rather than an empty pill', () => {
    const b = opBooking({ occupant_label: '  ', customer_display_name: '' });
    expect(occupantShortLabel(b, t)).toBeUndefined();
    expect(occupantFullName(b, t)).toBeUndefined();
  });

  it('captions only the spots taken for the slot, and none without permission', () => {
    const spots = [
      opSpot('s1', 'A1', 'free'),
      opSpot('s2', 'A2', 'booked', { booking: opBooking() }),
      opSpot('s3', 'A3', 'released', { booking: opBooking({ occupant_label: 'Verdi' }) }),
      opSpot('s4', 'A4', 'booked', {
        am_status: 'booked',
        pm_status: 'free',
        booking: opBooking({ occupant_label: 'Bianchi', slots: 'AM' }),
      }),
      opSpot('s5', 'A5', 'pass_holder', { booking: opBooking({ occupant_label: 'Neri' }) }),
    ];
    expect(occupantCaptions(spots, 'FULL', t, true)).toEqual({
      s2: 'Rossi',
      s4: 'Bianchi',
      s5: 'Neri',
    });
    expect(occupantCaptions(spots, 'PM', t, true)).toEqual({ s2: 'Rossi', s5: 'Neri' });
    expect(occupantCaptions(spots, 'FULL', t, false)).toEqual({});
  });
});

describe('useLabelMode', () => {
  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it('defaults to both and is remembered per user', () => {
    const { result, rerender } = renderHook(({ id }) => useLabelMode(id), {
      initialProps: { id: 'u1' as string | undefined },
    });
    expect(result.current[0]).toBe('both');
    act(() => result.current[1]('customer'));
    expect(result.current[0]).toBe('customer');
    expect(localStorage.getItem(labelModeKey('u1'))).toBe('customer');
    rerender({ id: 'u2' });
    expect(result.current[0]).toBe('both');
    rerender({ id: 'u1' });
    expect(result.current[0]).toBe('customer');
  });

  it('never persists under a shared key before the user is known', () => {
    localStorage.setItem(labelModeKey('u1'), 'code');
    const { result, rerender } = renderHook(({ id }) => useLabelMode(id), {
      initialProps: { id: undefined as string | undefined },
    });
    expect(result.current[0]).toBe('both');
    act(() => result.current[1]('customer'));
    expect(localStorage.getItem(labelModeKey(undefined))).toBeNull();
    // The user loads: their stored choice applies.
    rerender({ id: 'u1' });
    expect(result.current[0]).toBe('code');
  });

  it('ignores invalid stored values and survives a storage that throws', () => {
    localStorage.setItem(labelModeKey('u1'), 'banana');
    const { result } = renderHook(() => useLabelMode('u1'));
    expect(result.current[0]).toBe('both');
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceeded');
    });
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });
    act(() => result.current[1]('code'));
    expect(result.current[0]).toBe('code');
  });
});
