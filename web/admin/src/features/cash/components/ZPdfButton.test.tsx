import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../../i18n';
import { ZPdfButton } from './ZPdfButton';

function renderButton() {
  render(
    <I18nextProvider i18n={createAdminTestI18n()}>
      <ZPdfButton establishmentId="e1" sessionId="s1" />
    </I18nextProvider>,
  );
}

afterEach(() => {
  delete (window as { arenileDesktop?: unknown }).arenileDesktop;
});

describe('ZPdfButton', () => {
  it('offers "open in a new tab" in the browser', async () => {
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Altri formati della chiusura Z' }));
    expect(await screen.findByRole('menuitem', { name: 'Apri in una nuova scheda' })).toBeTruthy();
  });

  it('has no tabs in the desktop app window (only printing)', async () => {
    (window as { arenileDesktop?: unknown }).arenileDesktop = { mode: 'local', shell: 'window' };
    renderButton();
    await userEvent.click(screen.getByRole('button', { name: 'Altri formati della chiusura Z' }));
    expect(await screen.findByRole('menuitem', { name: 'PDF A4' })).toBeTruthy();
    expect(screen.queryByRole('menuitem', { name: 'Apri in una nuova scheda' })).toBeNull();
  });
});
