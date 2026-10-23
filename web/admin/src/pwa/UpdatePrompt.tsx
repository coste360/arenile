import { useEffect, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { pwaController, type PwaController } from './store';

const UPDATE_TOAST_ID = 'pwa-update';

export interface UpdatePromptProps {
  controller?: PwaController;
}

/**
 * Asks before activating a new version of the panel (sonner toast with an
 * action, kept until answered) and says once when the panel is ready to
 * work offline. Renders nothing itself.
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
    toast.success(t('update.offlineReady'));
    controller.ackOfflineReady();
  }, [state.offlineReady, controller, t]);

  return null;
}
