import { describe, expect, it } from 'vitest';
import { ticketMapLabel, ticketNumberLabel } from './lib';

describe('ticketNumberLabel', () => {
  it('pads to three digits', () => {
    expect(ticketNumberLabel(1)).toBe('#001');
    expect(ticketNumberLabel(37)).toBe('#037');
    expect(ticketNumberLabel(1204)).toBe('#1204');
  });
});

describe('ticketMapLabel', () => {
  it('prefers the holder name, else the number', () => {
    expect(ticketMapLabel({ ticket_number: 37 })).toBe('#037');
    expect(ticketMapLabel({ ticket_number: 37, holder_name: ' Rossi ' })).toBe('Rossi');
  });
});
