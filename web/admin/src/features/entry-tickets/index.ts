// Public surface of the entry tickets feature (spec 14.3, ADR 0020): the
// router imports the pages; "Vendita rapida" mounts EntrySalePanel in its
// "Ingressi" tab.
export { EntryGatePage } from './EntryGatePage';
export { EntrySalePanel, type EntrySalePanelProps } from './EntrySalePanel';
export { EntryTicketTypesPage } from './EntryTicketTypesPage';
