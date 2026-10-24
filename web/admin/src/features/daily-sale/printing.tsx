import { useCallback, useState, type ReactNode } from 'react';
import { createPortal, flushSync } from 'react-dom';
import { useCurrentEstablishment } from '../../establishment';
import type { TicketPrintData } from './api';
import { TicketPrint } from './TicketPrint';

const HIDDEN = 'print:hidden';
let restorePrevious: (() => void) | null = null;

/**
 * Prints only `target`: every other child of <body> (the app, open dialogs)
 * gets the `print:hidden` utility until the browser fires `afterprint`. No
 * stylesheet or inline style is involved (ADR 0009): only Tailwind classes.
 */
export function printIsolated(target: Element): void {
  restorePrevious?.();
  const hidden: Element[] = [];
  for (const el of Array.from(document.body.children)) {
    if (el.contains(target) || el.classList.contains(HIDDEN)) continue;
    el.classList.add(HIDDEN);
    hidden.push(el);
  }
  const restore = () => {
    for (const el of hidden) el.classList.remove(HIDDEN);
    window.removeEventListener('afterprint', restore);
    if (restorePrevious === restore) restorePrevious = null;
  };
  restorePrevious = restore;
  window.addEventListener('afterprint', restore);
  window.print();
}

/**
 * Ticket printer of a page: `print(data)` renders the ticket into a
 * print-only area at the end of <body> and opens the browser print dialog
 * (thermal printers are driven by the system print queue); `area` must be
 * rendered once by the caller.
 */
export function useTicketPrinter(): { print: (data: TicketPrintData) => void; area: ReactNode } {
  const [data, setData] = useState<TicketPrintData | null>(null);
  const { establishment } = useCurrentEstablishment();
  const print = useCallback((d: TicketPrintData) => {
    flushSync(() => setData(d));
    const target = document.querySelector('[data-ticket-print-area]');
    if (target) printIsolated(target);
  }, []);
  const area =
    data &&
    createPortal(
      <div className="hidden print:block" data-ticket-print-area="">
        <TicketPrint data={data} timeZone={establishment?.timezone} />
      </div>,
      document.body,
    );
  return { print, area };
}
