// Public surface of the daily sale feature ("Vendita giornaliera", spec 14.3,
// ADR 0020): the router, the day map, Oggi and Cassa import ONLY these names.
export { AssignPanel, type AssignPanelProps } from './AssignPanel';
export { useMoveTicket, useUnassignedTickets, type Ticket } from './api';
export { DailySaleForm, type DailySaleFormProps } from './DailySaleForm';
export { DailySalePage } from './DailySalePage';
export { TicketPrint, type TicketPrintProps } from './TicketPrint';
export { TicketPrintDialog, type TicketPrintDialogProps } from './TicketPrintDialog';
export { TicketsPage } from './TicketsPage';
export { useTicketAssignment, type TicketAssignment } from './useTicketAssignment';
export {
  VenditaGiornalieraSheet,
  type VenditaGiornalieraSheetProps,
} from './VenditaGiornalieraSheet';
export { ticketMapLabel, ticketNumberLabel } from './lib';
