import { expect, test } from '@playwright/test';
import { establishmentSwitcher, login } from './helpers';

/**
 * M1/M2 acceptance through the panel (spec 28): an owner creates a new
 * establishment from the switcher, applies the classic 8 × 15 template with
 * the guided setup, generates the quick price list, moves a spot in the map
 * editor and saves, then reads a quote in the price simulator.
 */

const PRICES: Record<string, number> = { row1: 30, row2: 26, standard: 22, gazebo: 60 };
const PERIOD_EXTRA: Record<string, number> = { low: 0, mid: 5, high: 10, peak: 18 };

test('owner configures a new lido from the classic template, edits the map and simulates a price', async ({
  page,
}) => {
  test.setTimeout(120_000);
  await login(page);

  // Establishment switcher (shadcn DropdownMenu in the sidebar) → new establishment.
  const switcher = establishmentSwitcher(page);
  await expect(switcher).toBeEnabled();
  await switcher.click();
  await page.getByRole('menuitem', { name: 'Nuovo stabilimento' }).click();
  await expect(page.getByRole('heading', { name: 'Nuovo stabilimento' })).toBeVisible();
  // Unique per run and per worker: the slug is derived from the name and must not collide.
  const name = `Lido E2E ${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  await page.getByLabel('Nome', { exact: true }).fill(name);
  await page.getByRole('button', { name: 'Crea stabilimento' }).click();

  // The guided setup opens on the new establishment.
  await expect(page).toHaveURL(/\/admin\/e\/[0-9a-f-]{36}\/setup/);
  const establishmentId = /\/e\/([0-9a-f-]{36})\//.exec(page.url())?.[1] as string;
  await expect(establishmentSwitcher(page)).toHaveAccessibleName(`Stabilimento: ${name}`);
  await establishmentSwitcher(page).click();
  await expect(page.getByRole('menuitemradio', { name, checked: true })).toBeVisible();
  await page.keyboard.press('Escape');

  // Step 1 (details) is optional.
  await page.getByRole('button', { name: 'Salta questo passo' }).click();

  // Step 2: classic template (one shadcn Field group per template).
  const classic = page
    .getByRole('radiogroup', { name: 'Modelli disponibili' })
    .getByRole('group')
    .filter({ hasText: 'Lido classico a file' })
    .first();
  await classic.getByRole('button', { name: 'Usa questo modello' }).click();

  // Step 3: 8 rows × 15 spots (defaults), season and map are created.
  await expect(page.getByLabel('File', { exact: true })).toHaveValue('8');
  await expect(page.getByLabel('Posti per fila')).toHaveValue('15');
  await page.getByRole('button', { name: 'Crea stagione e mappa' }).click();
  await expect(page.getByText('Configurazione creata.')).toBeVisible();
  await page.getByRole('button', { name: 'Continua' }).click();

  // Step 4: quick price list, one daily price per category and period.
  const cells = page.getByRole('textbox', { name: /^Prezzo giornaliero / });
  await expect(cells.first()).toBeVisible();
  const count = await cells.count();
  expect(count).toBeGreaterThanOrEqual(12);
  for (let i = 0; i < count; i++) {
    const cell = cells.nth(i);
    const label = (await cell.getAttribute('aria-label')) ?? '';
    const m = /Prezzo giornaliero (\S+), periodo (\S+)/.exec(label);
    const euro = (PRICES[m?.[1] ?? ''] ?? 20) + (PERIOD_EXTRA[m?.[2] ?? ''] ?? 0);
    await cell.fill(String(euro));
  }
  await page.getByRole('button', { name: 'Genera le regole di prezzo' }).click();
  await expect(page.getByText(/regole? di prezzo \(sostituite: 0\)/)).toBeVisible();

  // Map editor: 8 × 15 umbrellas (plus the template's gazebo row).
  await page.goto(`/admin/e/${establishmentId}/maps`);
  await page.getByRole('link', { name: 'Apri la mappa Spiaggia' }).click();
  const canvas = page.getByRole('group', { name: /^Mappa con \d+ postazioni$/ });
  await expect(canvas).toBeVisible();
  const labels = await canvas.locator('[data-spot-label]').evaluateAll((els) =>
    els.map((el) => el.getAttribute('data-spot-label') ?? ''),
  );
  expect(labels.filter((l) => /^[A-H]\d+$/.test(l))).toHaveLength(120);
  await expect(page.getByText(/^124 postazioni/)).toBeVisible();

  // Move A1 with the mouse (drag) and with the keyboard, then save.
  const a1 = canvas.getByRole('button', { name: /^Postazione A1,/ });
  const x0 = Number(await a1.getAttribute('data-x'));
  const box = await a1.boundingBox();
  if (!box) throw new Error('A1 not rendered');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 25, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(a1).toHaveAttribute('aria-pressed', 'true');
  const x1 = Number(await a1.getAttribute('data-x'));
  expect(x1).toBeGreaterThan(x0);
  await a1.focus();
  await page.keyboard.press('ArrowRight');
  await expect(a1).toHaveAttribute('data-x', String(x1 + 10));
  await page.getByRole('button', { name: 'Salva mappa' }).click();
  await expect(page.getByText(/^124 postazioni · salvata$/)).toBeVisible();

  // The position is persisted.
  await page.reload();
  await expect(
    page.getByRole('button', { name: /^Postazione A1,/ }),
  ).toHaveAttribute('data-x', String(x1 + 10));

  // Price simulator: a quote with the step-by-step breakdown.
  await page.goto(`/admin/e/${establishmentId}/pricing/simulator`);
  await expect(page.getByRole('heading', { name: 'Simulatore prezzi' })).toBeVisible();
  await page.getByRole('button', { name: 'Calcola' }).click();
  const result = page.getByRole('region', { name: 'Risultato della simulazione' });
  await expect(result).toBeVisible();
  // Defaults: first pack, first zone (row1), one day on the opening date (low period):
  // the quick price list above gives 30 € for row1 + 0 € for 'low'.
  await expect(result.getByTestId('simulator-total')).toHaveText(/^30,00\s€$/);
  const applied = result.getByRole('list', { name: 'Regole applicate' }).first();
  await expect(applied).toBeVisible();
  await expect(applied.getByRole('listitem').filter({ hasText: /Listino rapido/ })).toHaveCount(1);
  await expect(applied.getByRole('listitem').filter({ hasText: /Listino rapido/ })).toContainText(/30,00\s€/);
});
