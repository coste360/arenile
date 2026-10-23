// Public surface of the printers feature (ESC/POS network printers, ADR
// 0021): the router imports ONLY these names. The printers builder owns
// everything inside features/printers/ (page, hooks, the print-or-fallback
// helper used by the cash buttons).
export { PrintersPage } from './PrintersPage';
