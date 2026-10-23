import type { ComponentType, ReactElement } from 'react';
import { useInstanceMeta, panelMode } from '../../lib/meta';
import { DesktopOnlineNotice, type DesktopOnlineTopic } from './DesktopOnlineNotice';

/**
 * Wraps the page of an online-only section (public page, online payments):
 * on the desktop edition (local or LAN) it renders DesktopOnlineNotice
 * instead (decorator: the online pages stay unaware of the edition). While
 * /meta loads nothing is rendered, so neither variant flashes.
 */
export function desktopGated(
  topic: DesktopOnlineTopic,
  Page: ComponentType,
): () => ReactElement | null {
  function DesktopGated(): ReactElement | null {
    const meta = useInstanceMeta();
    if (meta.isPending) return null;
    const mode = panelMode(meta.data);
    if (mode === 'local' || mode === 'lan') return <DesktopOnlineNotice topic={topic} />;
    return <Page />;
  }
  DesktopGated.displayName = `DesktopGated(${topic})`;
  return DesktopGated;
}
