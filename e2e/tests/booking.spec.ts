import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { DEMO_SLUG, demoEstablishment } from './helpers';

test('/b/lido-demo shows the establishment name', async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  await expect(page).toHaveTitle(new RegExp(name));
  await expect(page.getByRole('button', { name: /Quando/ })).toBeVisible();

  // "Quando": shadcn date range picker (lazy), dd/MM/yyyy, never a native date input.
  const dates = page.getByRole('button', { name: /Date del soggiorno/ });
  await expect(dates).toBeVisible();
  await expect(page.locator('input[type="date"]')).toHaveCount(0);
  await dates.click();
  const grid = page.getByRole('grid').first();
  await expect(grid).toBeVisible();
  await grid.locator('button:not([disabled])').first().click();
  await expect(dates).toHaveText(/\d{2}\/\d{2}\/\d{4} – /);
});

test('/b/lido-demo has no serious accessibility violations', async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
  const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test('/b/lido-demo works at 320px width', async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(overflow).toBe(false);
});

test('unknown slug shows not found', async ({ page }) => {
  await page.goto('/b/questo-lido-non-esiste');
  await expect(page.getByRole('alert')).toHaveText('Stabilimento non trovato.');
});
