// Public surface of the daily sale feature ("Vendita giornaliera", spec 14.3,
// ADR 0020): the router, the day map, Oggi and Cassa import ONLY these names.
export { AssignPanel, type AssignPanelProps } from './AssignPanel';
export { DailySalePage } from './DailySalePage';
export { TicketPrint, type TicketPrintProps } from './TicketPrint';
export { TicketsPage } from './TicketsPage';
export {
  VenditaGiornalieraSheet,
  type VenditaGiornalieraSheetProps,
} from './VenditaGiornalieraSheet';
export { ticketMapLabel, ticketNumberLabel } from './lib';
