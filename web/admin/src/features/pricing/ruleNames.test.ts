import { describe, expect, it } from 'vitest';
import { formatEffect } from './ruleNames';

const money = (c: number) => `${(c / 100).toFixed(2).replace('.', ',')} €`;

describe('formatEffect', () => {
  it('formats the engine effects as money and percentages', () => {
    expect(formatEffect('set 2500', money, 'it-IT')).toBe('= 25,00 €');
    expect(formatEffect('+300', money, 'it-IT')).toBe('+3,00 €');
    expect(formatEffect('-500', money, 'it-IT')).toBe('−5,00 €');
    expect(formatEffect('x1.2', money, 'it-IT')).toBe('× 1,2');
    expect(formatEffect('-12.5%', money, 'it-IT')).toBe('−12,5%');
  });

  it('keeps unknown effects as they are', () => {
    expect(formatEffect('cap 3', money, 'it-IT')).toBe('cap 3');
  });
});
