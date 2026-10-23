import { Toaster } from '@arenile/shared/ui/sonner';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';
import { createAdminTestI18n } from '../i18n';
import { createPwaController } from './store';
import { UpdatePrompt } from './UpdatePrompt';

function setup() {
  const controller = createPwaController();
  const update = vi.fn(async () => {});
  controller.bindUpdater(update);
  render(
    <I18nextProvider i18n={createAdminTestI18n()}>
      <UpdatePrompt controller={controller} />
      <Toaster />
    </I18nextProvider>,
  );
  return { controller, update };
}

describe('UpdatePrompt', () => {
  it('asks before activating a new version and updates on demand', async () => {
    const user = userEvent.setup();
    const { controller, update } = setup();
    expect(screen.queryByText('È disponibile una nuova versione del pannello.')).toBeNull();
    act(() => controller.setNeedRefresh());
    expect(
      await screen.findByText('È disponibile una nuova versione del pannello.'),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Aggiorna' }));
    expect(update).toHaveBeenCalledTimes(1);
  });

  it('can be postponed', async () => {
    const user = userEvent.setup();
    const { controller, update } = setup();
    act(() => controller.setNeedRefresh());
    await user.click(await screen.findByRole('button', { name: 'Più tardi' }));
    expect(update).not.toHaveBeenCalled();
    expect(controller.getSnapshot().needRefresh).toBe(false);
  });

  it('says once that the panel works offline', async () => {
    const { controller } = setup();
    act(() => controller.setOfflineReady());
    expect(
      await screen.findByText('Il pannello è pronto per funzionare anche offline.'),
    ).toBeInTheDocument();
    await waitFor(() => expect(controller.getSnapshot().offlineReady).toBe(false));
  });
});
