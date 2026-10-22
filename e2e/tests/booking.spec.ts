import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { DEMO_SLUG, demoEstablishment } from './helpers';

test('/b/lido-demo shows the establishment name', async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  await expect(page).toHaveTitle(new RegExp(name));
  await expect(page.getByRole('button', { name: /Quando/ })).toBeVisible();
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
