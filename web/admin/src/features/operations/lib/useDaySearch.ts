import { useNavigate, useSearch } from '@tanstack/react-router';
import { useCallback } from 'react';
import { useCurrentEstablishment } from '../../../establishment';
import { isIsoDate, todayIn } from './dates';
import type { SlotChoice } from './status';

export interface DaySelection {
  /** Selected day ("YYYY-MM-DD", establishment time zone). */
  date: string;
  slot: SlotChoice;
  /** Today in the establishment time zone. */
  today: string;
  /** Updates `?date&slot` (the URL is the source of truth: shareable, back button). */
  setDay: (next: { date?: string; slot?: SlotChoice }) => void;
}

/** Day and slot of the operational pages from `?date=&slot=` (router validateDaySearch). */
export function useDaySearch(): DaySelection {
  const search = useSearch({ strict: false }) as { date?: unknown; slot?: unknown };
  const navigate = useNavigate();
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const date = isIsoDate(search.date) ? search.date : today;
  const slot: SlotChoice =
    search.slot === 'AM' || search.slot === 'PM' || search.slot === 'FULL' ? search.slot : 'FULL';
  const setDay = useCallback(
    (next: { date?: string; slot?: SlotChoice }) => {
      void navigate({
        to: '.',
        search: ((prev: Record<string, unknown>) => ({ ...prev, ...next })) as never,
        replace: true,
      });
    },
    [navigate],
  );
  return { date, slot, today, setDay };
}
