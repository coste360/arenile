import {
  call,
  shouldRetry,
  useApiClient,
  type ApiClient,
  type components,
} from '@arenile/shared/api';
import { useQuery } from '@tanstack/react-query';

export type InstanceMeta = components['schemas']['Meta'];

/**
 * How this panel is being used (spec 29.0/29.1):
 * - `server`: browser on a server instance (VPS, server in sede);
 * - `local`: desktop edition, browser on the cash desk PC (127.0.0.1);
 * - `lan`: desktop edition with the LAN mode on;
 * - `remote`: the panel of a server instance inside the desktop app window
 *   (remote mode, fiscal bridge). The server cannot know this: the desktop
 *   window injects `window.arenileDesktop = { mode: 'remote' }`.
 */
export type PanelMode = 'server' | 'local' | 'lan' | 'remote';

/** Global injected by the desktop app window (remote mode only). */
interface DesktopBridgeGlobal {
  arenileDesktop?: { mode?: string };
}

export const metaQueryKey = ['instance', 'meta'] as const;

export function metaQueryOptions(client: ApiClient) {
  return {
    queryKey: metaQueryKey,
    queryFn: () => call(() => client.GET('/api/admin/v1/meta')),
    retry: shouldRetry,
    // The desktop mode changes only when the operator toggles the LAN mode.
    staleTime: 5 * 60_000,
  };
}

/** Public instance metadata (GET /api/admin/v1/meta). */
export function useInstanceMeta() {
  const client = useApiClient();
  return useQuery(metaQueryOptions(client));
}

/** Pure mapping from the metadata (and the window global) to the panel mode. */
export function panelMode(
  meta: Pick<InstanceMeta, 'edition' | 'desktop'> | undefined,
  global: DesktopBridgeGlobal = globalThis as DesktopBridgeGlobal,
): PanelMode {
  if (global.arenileDesktop?.mode === 'remote') return 'remote';
  if (meta?.edition !== 'desktop') return 'server';
  return meta.desktop?.mode === 'lan' ? 'lan' : 'local';
}

/**
 * Panel mode for the current instance; `server` while loading, so the
 * server UI never flashes desktop-only notices. Owner of the desktop
 * behaviour of the pages: desktop runtime builder (public page and online
 * payments disabled with an explanation, spec 29.1 "Limiti voluti").
 */
export function usePanelMode(): PanelMode {
  const { data } = useInstanceMeta();
  return panelMode(data);
}

/**
 * True inside the desktop app window (local or remote mode: the window
 * injects `window.arenileDesktop`). There every link that leaves the panel
 * opens in the default browser, and a server document opened in a new
 * window is saved as a download instead of being shown.
 */
export function inDesktopWindow(
  global: DesktopBridgeGlobal = globalThis as DesktopBridgeGlobal,
): boolean {
  return global.arenileDesktop != null;
}

/** True on the desktop edition (local or LAN). */
export function useIsDesktopEdition(): boolean {
  const mode = usePanelMode();
  return mode === 'local' || mode === 'lan';
}
