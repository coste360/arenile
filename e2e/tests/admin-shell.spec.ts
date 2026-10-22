import { expect, test } from '@playwright/test';
import { login } from './helpers';

/** Panel shell (shadcn): light/dark theme from the user menu and the Ctrl+K command palette. */

test('the user menu switches to dark mode and the choice survives a reload', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await login(page);
  const html = page.locator('html');
  await expect(html).not.toHaveClass(/(^|\s)dark(\s|$)/);

  await page.getByRole('button', { name: 'Menu utente' }).click();
  await page.getByRole('menuitemradio', { name: 'Scuro' }).click();
  await expect(html).toHaveClass(/(^|\s)dark(\s|$)/);

  await page.reload();
  await expect(html).toHaveClass(/(^|\s)dark(\s|$)/);

  await page.getByRole('button', { name: 'Menu utente' }).click();
  await page.getByRole('menuitemradio', { name: 'Chiaro' }).click();
  await expect(html).not.toHaveClass(/(^|\s)dark(\s|$)/);
});

test('Ctrl+K opens the command palette and navigates to a page', async ({ page }) => {
  await login(page);
  await expect(page.getByRole('button', { name: /^Stabilimento: / })).toBeVisible();

  await page.keyboard.press('Control+k');
  const palette = page.getByRole('dialog', { name: 'Comandi rapidi' });
  await expect(palette).toBeVisible();
  await expect(palette.getByRole('combobox')).toBeFocused();

  await page.keyboard.type('Tutti gli stabilimenti');
  await palette.getByRole('option', { name: 'Tutti gli stabilimenti', exact: true }).click();
  await expect(palette).toBeHidden();
  await expect(page).toHaveURL(/\/admin\/configurazione/);

  // Escape closes it again.
  await page.keyboard.press('Control+k');
  await expect(palette).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(palette).toBeHidden();
});
