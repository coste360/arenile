// Public surface of the printers feature (ESC/POS network printers, ADR
// 0021): the router imports the page; the cash buttons import the
// print-or-fallback helpers; the desktop settings link here (PrintersHint).
// Everything else stays inside features/printers/.
export { PrintersPage } from './PrintersPage';
export { usePrintDocument, type PrintRequest } from './usePrintDocument';
export { usePrintReady } from './usePrintReady';
export { PrintersHint } from './PrintersHint';
