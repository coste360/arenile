import { useCallback, useState, type ReactNode } from 'react';
import { createPortal, flushSync } from 'react-dom';
import { useCurrentEstablishment } from '../../establishment';
import { usePrintDocument, usePrintReady } from '../printers';
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

/** Loads the browser print data of a ticket (only when the browser prints it). */
export type TicketPrintLoader = () => TicketPrintData | Promise<TicketPrintData>;

/**
 * Ticket output of a page (ADR 0021): when the daily_ticket route resolves
 * to a ready ESC/POS printer (desktop edition, printer active and reachable)
 * `print(ticketId, load)` queues a thermal job, with the browser print as
 * the fallback if the job cannot be queued; otherwise it prints through the
 * browser exactly as before (`useTicketPrinter`). `load` is called only for
 * the browser print; `browserOnly` forces the browser (a sheet format
 * picked in the preview). `area` must be rendered once by the caller. The
 * returned promise rejects when the browser print data cannot be loaded;
 * inside the thermal fallback the same failure goes to `onError`.
 */
export function useTicketOutput(registerId?: string): {
  print: (
    ticketId: string,
    load: TicketPrintLoader,
    onError?: (error: unknown) => void,
    browserOnly?: boolean,
  ) => Promise<void>;
  area: ReactNode;
  thermal: boolean;
} {
  const browser = useTicketPrinter();
  const thermal = usePrintReady('daily_ticket', registerId);
  const printDocument = usePrintDocument();
  const browserPrint = browser.print;
  const print = useCallback(
    async (
      ticketId: string,
      load: TicketPrintLoader,
      onError?: (error: unknown) => void,
      browserOnly = false,
    ) => {
      if (!thermal || browserOnly) {
        browserPrint(await load());
        return;
      }
      await printDocument({ kind: 'daily_ticket', documentId: ticketId, registerId }, () => {
        void Promise.resolve()
          .then(load)
          .then(browserPrint)
          .catch((err: unknown) => onError?.(err));
      });
    },
    [thermal, printDocument, registerId, browserPrint],
  );
  return { print, area: browser.area, thermal };
}
