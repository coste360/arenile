import { useEffect, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { pwaController, type PwaController } from './store';

const UPDATE_TOAST_ID = 'pwa-update';
/** localStorage flag: the "ready offline" notice was already shown on this browser. */
export const OFFLINE_READY_SHOWN_KEY = 'arenile-admin:offline-ready-shown';

function offlineReadyAlreadyShown(): boolean {
  try {
    return localStorage.getItem(OFFLINE_READY_SHOWN_KEY) === '1';
  } catch {
    return false;
  }
}

function rememberOfflineReadyShown(): void {
  try {
    localStorage.setItem(OFFLINE_READY_SHOWN_KEY, '1');
  } catch {
    // Private window or blocked storage: the notice may show again, nothing worse.
  }
}

export interface UpdatePromptProps {
  controller?: PwaController;
}

/**
 * Asks before activating a new version of the panel (sonner toast with an
 * action, kept until answered) and says once per browser, the first time
 * the service worker finishes precaching, that the panel is ready to work
 * offline. That notice sits bottom-right so it never covers the connection
 * indicator in the header (spec 21.2: always visible). Renders nothing
 * itself.
 */
export function UpdatePrompt({ controller = pwaController }: UpdatePromptProps) {
  const { t } = useTranslation('offline');
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot);

  useEffect(() => {
    if (!state.needRefresh) return;
    toast(t('update.available'), {
      id: UPDATE_TOAST_ID,
      duration: Infinity,
      action: { label: t('update.reload'), onClick: () => void controller.update() },
      cancel: { label: t('update.later'), onClick: () => controller.dismiss() },
    });
  }, [state.needRefresh, controller, t]);

  useEffect(() => {
    if (!state.offlineReady) return;
    if (!offlineReadyAlreadyShown()) {
      toast.success(t('update.offlineReady'), { position: 'bottom-right' });
      rememberOfflineReadyShown();
    }
    controller.ackOfflineReady();
  }, [state.offlineReady, controller, t]);

  return null;
}
