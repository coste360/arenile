// Public surface of the find-spot feature (spec 30.6, ADR 0028): the page
// for the router (lazy import), the panel and the sheet the map and the new
// booking open with "Trova posto".
export { FindSpotPage } from './FindSpotPage';
export { FindSpotPanel, type FindSpotPanelProps } from './FindSpotPanel';
export { FindSpotSheet, type FindSpotSheetProps } from './FindSpotSheet';
export { FindSpotBookDialog, type FindSpotBookDialogProps } from './FindSpotBookDialog';
export { segmentsText } from './lib';
export type { FindSpotProposal, FindSpotRequest } from './api';
export type { FindSpotDefaults } from './types';
export { FindSpotLauncher, type FindSpotLauncherProps } from './FindSpotLauncher';
