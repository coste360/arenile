// Public surface of the desktop feature (M6, spec 29.1): the router imports
// ONLY these names (lazily), plus the small eager entry ./desktopGated used
// to wrap the online-only routes. The desktop launcher builder owns
// features/desktop/; the first-run page lives in features/first-run/.
export { DesktopSettingsPage } from './DesktopSettingsPage';
export { DesktopOnlineNotice, type DesktopOnlineTopic } from './DesktopOnlineNotice';
