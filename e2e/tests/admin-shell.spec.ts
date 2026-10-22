import AxeBuilder from '@axe-core/playwright';
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

/** Serious/critical axe violations (WCAG 2.1 AA) on a page, after it settled. */
async function seriousViolations(page: import('@playwright/test').Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  return results.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')})`);
}

for (const theme of ['light', 'dark'] as const) {
  test(`the shell, a form page and the map editor have no serious accessibility violations (${theme})`, async ({
    page,
  }) => {
    test.setTimeout(90_000);
    await login(page);
    await page.evaluate((t) => localStorage.setItem('arenile-theme', t), theme);
    await page.reload();
    await expect(page.locator('html')).toHaveClass(theme === 'dark' ? /(^|\s)dark(\s|$)/ : /^(?!.*\bdark\b)/);

    // Shell: establishments list.
    await page.goto('/admin/configurazione');
    await expect(page.getByRole('heading', { name: 'Tutti gli stabilimenti' })).toBeVisible();
    expect(await seriousViolations(page), 'shell').toEqual([]);

    // The current establishment's id, from its configuration link in the sidebar.
    const settingsLink = page
      .getByRole('navigation', { name: 'Configurazione dello stabilimento' })
      .getByRole('link', { name: 'Stabilimento', exact: true });
    const establishmentId = /\/e\/([0-9a-f-]{36})/.exec((await settingsLink.getAttribute('href')) ?? '')?.[1];
    expect(establishmentId, 'establishment id from the sidebar').toBeTruthy();

    // A form page: establishment settings.
    await page.goto(`/admin/e/${establishmentId}/config/establishment`);
    await expect(page.getByRole('region', { name: 'Dati generali' })).toBeVisible();
    expect(await seriousViolations(page), 'settings form').toEqual([]);

    // The map editor.
    await page.goto(`/admin/e/${establishmentId}/maps`);
    await page.getByRole('link', { name: /Apri la mappa/ }).first().click();
    await expect(page.getByRole('group', { name: /^Mappa con \d+ postazioni$/ })).toBeVisible();
    expect(await seriousViolations(page), 'map editor').toEqual([]);
  });
}
