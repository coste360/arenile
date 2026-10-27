import { useCallback, useState, type ReactNode } from 'react';
import { createPortal, flushSync } from 'react-dom';
import { printIsolated } from '../../daily-sale/printing';
import { DayMapSheet, type DayMapPrintData } from './DayMapSheet';

/**
 * Printed map of the day (spec 30.10): date and slot, the map with the
 * chosen labels, legend and totals (occupied, free, arrivals, departures).
 * Rendered into a print-only area at the end of <body> and printed with the
 * browser (works offline, no external service, only Tailwind print
 * utilities). The paper size and the landscape orientation are chosen in
 * the browser print dialog; the drawing is sized for the paper chosen here.
 */
export function useDayMapPrint(): { print: (data: DayMapPrintData) => void; area: ReactNode } {
  const [data, setData] = useState<DayMapPrintData | null>(null);
  const print = useCallback((d: DayMapPrintData) => {
    flushSync(() => setData(d));
    const target = document.querySelector('[data-day-map-print]');
    if (target) printIsolated(target);
  }, []);
  const area =
    data &&
    createPortal(
      <div className="hidden print:block" data-day-map-print="">
        <DayMapSheet data={data} />
      </div>,
      document.body,
    );
  return { print, area };
}
