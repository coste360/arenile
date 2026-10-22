import { expect, test } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DEMO_SLUG, demoEstablishment } from './helpers';

const hostHtml = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', 'fixtures', 'host.html'), 'utf8');

test('embed.js on a host page creates the iframe and resizes it', async ({ page, request, baseURL }) => {
  const { name } = await demoEstablishment(request);
  // Serve the fixture from the instance origin so the default
  // `frame-ancestors 'self'` policy (empty allowed_embed_origins) allows it.
  const hostUrl = `${baseURL}/__e2e/host.html`;
  await page.route(hostUrl, (route) =>
    route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: hostHtml }),
  );
  await page.goto(hostUrl);

  const iframe = page.locator(`[data-arenile="${DEMO_SLUG}"] iframe`);
  await expect(iframe).toHaveCount(1);
  await expect(iframe).toHaveAttribute('src', new RegExp(`/b/${DEMO_SLUG}\\?embed=1`));
  await expect(page.frameLocator(`[data-arenile="${DEMO_SLUG}"] iframe`).getByText(name).first()).toBeVisible();

  // arenile:resize from the iframe sets an explicit height and is re-dispatched.
  await expect.poll(() => iframe.evaluate((el) => (el as HTMLIFrameElement).style.height)).toMatch(/^\d+px$/);
  await expect.poll(() => page.evaluate(() => (window as unknown as { __resizes: number }).__resizes)).toBeGreaterThan(0);
});
