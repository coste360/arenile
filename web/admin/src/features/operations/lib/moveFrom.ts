import { addDays } from './dates';
import type { OperationalSpot } from './status';

/**
 * "Da quando" of a move or a swap (spec 30.7): the whole stay (when it has
 * not started yet), today, tomorrow or a chosen date. A future date is a
 * scheduled move: the server moves the future allocations at once.
 */
export type MoveFromChoice = 'whole' | 'today' | 'tomorrow' | 'date';

export interface StayWindow {
  starts_on: string;
  ends_on: string;
}

/** Choices that make sense for `stay` seen on `today` (in the establishment zone). */
export function moveFromChoices(stay: StayWindow, today: string): MoveFromChoice[] {
  const out: MoveFromChoice[] = [];
  if (stay.starts_on >= today) out.push('whole');
  if (today > stay.starts_on && today <= stay.ends_on) out.push('today');
  const tomorrow = addDays(today, 1);
  if (tomorrow > stay.starts_on && tomorrow <= stay.ends_on) out.push('tomorrow');
  if (stay.starts_on < stay.ends_on) out.push('date');
  return out;
}

/** Default choice: the day shown on the map when it is inside the stay, else the first possible. */
export function defaultMoveFrom(
  stay: StayWindow,
  today: string,
  shown: string,
): { choice: MoveFromChoice; date?: string } {
  const choices = moveFromChoices(stay, today);
  if (shown > stay.starts_on && shown <= stay.ends_on) {
    if (shown === today && choices.includes('today')) return { choice: 'today' };
    if (shown === addDays(today, 1) && choices.includes('tomorrow')) return { choice: 'tomorrow' };
    return { choice: 'date', date: shown };
  }
  return { choice: choices[0] ?? 'whole' };
}

/**
 * First day moved for a choice; undefined = the whole line (no `from`).
 * Dates on or before the start of the stay mean the whole line.
 */
export function moveFromDate(
  choice: MoveFromChoice,
  stay: StayWindow,
  today: string,
  date?: string,
): string | undefined {
  let from: string | undefined;
  if (choice === 'today') from = today;
  else if (choice === 'tomorrow') from = addDays(today, 1);
  else if (choice === 'date') from = date;
  if (!from || from <= stay.starts_on) return undefined;
  return from > stay.ends_on ? stay.ends_on : from;
}

/** Common stay of the two lines on their spots (segments of a split stay included). */
export function commonWindow(a: OperationalSpot, b: OperationalSpot, date: string): StayWindow {
  const ba = a.booking;
  const bb = b.booking;
  const startA = ba?.segment_starts_on ?? ba?.starts_on ?? date;
  const startB = bb?.segment_starts_on ?? bb?.starts_on ?? date;
  const endA = ba?.segment_ends_on ?? ba?.ends_on ?? date;
  const endB = bb?.segment_ends_on ?? bb?.ends_on ?? date;
  return {
    starts_on: startA > startB ? startA : startB,
    ends_on: endA < endB ? endA : endB,
  };
}
