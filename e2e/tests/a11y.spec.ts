import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { DEMO_SLUG, login } from "./helpers";
import { bookableDay } from "./demo-dates";

/**
 * Accessibility pass (WCAG 2.1 AA, European Accessibility Act for consumer
 * booking): axe on every step of the public flow, the outcome, the manage
 * area and the main panel pages; a booking completed with the keyboard
 * only; the language attribute and reduced motion. The demo establishment
 * pays at the desk (manual provider, no online payment), so the flow ends
 * on "Prenotazione confermata" without leaving Arenile.
 *
 * Each test books on its own June day: other specs take the first free
 * spots of theirs (see booking.spec.ts and daily-sale.spec.ts).
 */

const MAIL_DIR = join(process.env.ARENILE_E2E_DATA_DIR ?? "", "mail");
const AXE_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

/** Serious/critical axe violations on the page, after animations settled. */
async function seriousViolations(
  page: Page,
  opts: { disable?: string[]; exclude?: string } = {},
): Promise<string[]> {
  // axe samples colours mid-transition otherwise; endless animations
  // (spinners, skeletons) never settle and are ignored.
  await page.waitForFunction(() =>
    document
      .getAnimations()
      .every(
        (a) => a.playState !== "running" || a.effect?.getComputedTiming().iterations === Infinity,
      ),
  );
  let builder = new AxeBuilder({ page }).withTags(AXE_TAGS);
  if (opts.disable?.length) builder = builder.disableRules(opts.disable);
  if (opts.exclude) builder = builder.exclude(opts.exclude);
  const results = await builder.analyze();
  return results.violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map(
      (v) =>
        `${v.id}: ${v.help} → ${v.nodes
          .slice(0, 3)
          .map((n) => `${n.target.join(" ")} ${n.failureSummary ?? ""}`)
          .join(" | ")}`,
    );
}

/** Opens the calendar, picks one free season day (twice = a single day) and closes it. */
async function pickDay(page: Page, n: number) {
  const target = bookableDay(n);
  await page.getByRole("button", { name: /Date del soggiorno/ }).click();
  const grid = page.getByRole("grid").first();
  await expect(grid).toBeVisible();
  for (let i = 0; i < 12; i++) {
    if (target.month.test((await grid.getAttribute("aria-label")) ?? "")) break;
    await page.getByRole("button", { name: /mese successivo|Next/i }).click();
  }
  const name = target.name;
  await grid.getByRole("button", { name }).click();
  await page.getByRole("grid").first().getByRole("button", { name }).click();
  await expect(grid).toBeHidden();
}

/** Waits for the customer email and returns the manage link it carries. */
async function manageLink(email: string): Promise<string> {
  let link = "";
  await expect
    .poll(
      () => {
        let files: string[] = [];
        try {
          files = readdirSync(MAIL_DIR).filter((f) => f.endsWith(".eml"));
        } catch {
          return "";
        }
        for (const f of files) {
          // Quoted-printable: soft line breaks and "=3D" for "=".
          const raw = readFileSync(join(MAIL_DIR, f), "utf8")
            .replace(/=\r?\n/g, "")
            .replace(/=3D/g, "=");
          if (!raw.includes(email)) continue;
          const m = new RegExp(`/b/${DEMO_SLUG}/m/[A-Za-z0-9_\\-.~]+`).exec(raw);
          if (m) link = m[0];
        }
        return link;
      },
      { timeout: 60_000, message: `manage link emailed to ${email}` },
    )
    .not.toBe("");
  return link;
}

test.describe("public booking page", () => {
  test("every step, the outcome and the manage area have no serious violations", async ({
    page,
  }) => {
    test.setTimeout(180_000);
    await page.goto(`/b/${DEMO_SLUG}`);
    await expect(page.getByRole("button", { name: /Date del soggiorno/ })).toBeVisible();
    expect(await seriousViolations(page), "1 Quando").toEqual([]);

    // The calendar popover: a named dialog with the availability legend.
    await page.getByRole("button", { name: /Date del soggiorno/ }).click();
    await expect(page.getByRole("dialog", { name: "Date del soggiorno" })).toBeVisible();
    await expect(page.getByRole("list", { name: "Disponibilità" })).toBeVisible();
    expect(await seriousViolations(page), "1 Quando (calendar)").toEqual([]);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: /Date del soggiorno/ })).toBeFocused();

    await pickDay(page, 9);
    expect(await seriousViolations(page), "1 Quando (dates chosen)").toEqual([]);
    await page.getByRole("button", { name: "Avanti" }).click();

    // 2. Dove, map view: spots are named buttons in a labelled group.
    await expect(page.getByRole("heading", { name: "2. Dove" })).toBeFocused();
    const map = page.getByRole("group", { name: "Spiaggia" });
    await expect(map).toBeVisible();
    await expect(map.getByRole("button", { name: /Libero/ }).first()).toBeAttached();
    expect(await seriousViolations(page), "2 Dove (map)").toEqual([]);

    // List view: the non-visual alternative to the map, grouped by row.
    await page.getByRole("button", { name: "Mostra come elenco" }).click();
    await expect(page.getByRole("region", { name: "Elenco delle postazioni" })).toBeVisible();
    // Each row is a named toolbar; a spot's name is its visible text.
    await expect(page.getByRole("toolbar", { name: /^Fila / }).first()).toBeVisible();
    expect(await seriousViolations(page), "2 Dove (list)").toEqual([]);
    await page
      .getByRole("button", { name: /^(?!.*(?:Gazebo|GZ))\S+ Libero/ })
      .first()
      .click();
    await page.getByRole("button", { name: "Avanti" }).click();

    // 3. Cosa.
    await expect(page.getByRole("heading", { name: "3. Cosa" })).toBeFocused();
    await page.getByRole("radio", { name: /^Base/ }).click();
    expect(await seriousViolations(page), "3 Cosa").toEqual([]);
    await page.getByRole("button", { name: "Continua" }).click();

    // 4. Chi e pagamento: empty submit shows associated errors and focuses the first.
    await expect(page.getByRole("heading", { name: "4. Chi e pagamento" })).toBeFocused();
    expect(await seriousViolations(page), "4 Chi").toEqual([]);
    await page.getByRole("button", { name: /^(Paga|Conferma|Prenota|Invia)/ }).click();
    const first = page.getByRole("textbox", { name: "Nome", exact: true });
    await expect(first).toBeFocused();
    await expect(first).toHaveAttribute("aria-invalid", "true");
    const describedBy = (await first.getAttribute("aria-describedby")) ?? "";
    expect(describedBy).not.toBe("");
    await expect(page.locator(`[id="${describedBy.split(" ")[0]}"]`)).not.toBeEmpty();
    await page.getByRole("button", { name: "Leggi le condizioni" }).click();
    expect(await seriousViolations(page), "4 Chi (errors, terms)").toEqual([]);

    const email = `a11y.axe.${Date.now()}@mailbox.test`;
    await first.fill("Alessia");
    await page.getByRole("textbox", { name: "Cognome" }).fill("Accessibile");
    await page.getByRole("textbox", { name: "Email" }).fill(email);
    await page.getByRole("checkbox", { name: /accetto le condizioni/ }).check();
    await page.getByRole("button", { name: /^(Paga|Conferma|Prenota|Invia)/ }).click();

    // Outcome: focus on its heading, announced.
    const outcome = page.getByRole("heading", { name: "Prenotazione confermata" });
    await expect(outcome).toBeFocused({ timeout: 30_000 });
    expect(await seriousViolations(page), "outcome").toEqual([]);

    // Manage area from the emailed link: details, then every action panel open.
    await page.goto(await manageLink(email));
    await expect(page.getByRole("heading", { level: 1, name: "La tua prenotazione" })).toBeVisible();
    expect(await seriousViolations(page), "manage").toEqual([]);
    // The demo policy lets a booking far in the future be modified and cancelled.
    for (const name of ["Cambia date", "Cancella la prenotazione"]) {
      const trigger = page.getByRole("button", { name, exact: true }).first();
      await expect(trigger).toBeVisible();
      await trigger.click();
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
    }
    // The refund preview button only shows when something was paid.
    const refund = page.getByRole("button", { name: "Vedi rimborso" });
    if ((await refund.count()) > 0) {
      await refund.click();
      await expect(refund).toBeHidden();
    }
    expect(await seriousViolations(page), "manage (panels open)").toEqual([]);

    // The cancel confirmation: a focus-trapped alert dialog (the second
    // "Cancella la prenotazione" button, inside its panel).
    const cancel = page.getByRole("button", { name: "Cancella la prenotazione", exact: true }).nth(1);
    await cancel.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(":focus")).toHaveCount(1);
    expect(await seriousViolations(page), "manage (cancel dialog)").toEqual([]);
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(cancel).toBeFocused();
  });

  test("invalid manage link and unknown establishment have no serious violations", async ({
    page,
  }) => {
    await page.goto(`/b/${DEMO_SLUG}/m/not-a-valid-token`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Email" })).toBeVisible();
    expect(await seriousViolations(page), "manage: invalid link").toEqual([]);

    await page.goto("/b/questo-lido-non-esiste");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(
      "Stabilimento non trovato.",
    );
    expect(await seriousViolations(page), "not found").toEqual([]);
  });

  test("the page language follows the language picker", async ({ page }) => {
    await page.goto(`/b/${DEMO_SLUG}`);
    await expect(page.locator("html")).toHaveAttribute("lang", "it");
    await page.getByRole("combobox", { name: "Lingua" }).selectOption("en");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expect(page.getByRole("button", { name: "Next" })).toBeVisible();
    await page.goto(`/b/${DEMO_SLUG}?lang=de`);
    await expect(page.locator("html")).toHaveAttribute("lang", "de");
  });

  test("animations are turned off when the device asks for reduced motion", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(`/b/${DEMO_SLUG}`);
    await page.getByRole("button", { name: /Date del soggiorno/ }).click();
    const popover = page.getByRole("dialog", { name: "Date del soggiorno" });
    await expect(popover).toBeVisible();
    const duration = await popover.evaluate((el) => getComputedStyle(el).animationDuration);
    expect(parseFloat(duration)).toBeLessThan(0.001);
  });
});

/** Accessible name of the focused element (aria-labelledby, aria-label, label, text). */
async function focusedName(page: Page): Promise<{ name: string; tag: string; visibleFocus: boolean }> {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { name: "", tag: "", visibleFocus: false };
    const text = (n: Element | null) => (n?.textContent ?? "").replace(/\s+/g, " ").trim();
    const ids = el.getAttribute("aria-labelledby");
    const name = ids
      ? ids
          .split(/\s+/)
          .map((id) => text(document.getElementById(id)))
          .join(" ")
      : (el.getAttribute("aria-label") ??
        ((el as HTMLInputElement).labels?.[0] ? text((el as HTMLInputElement).labels![0]!) : text(el)));
    const style = getComputedStyle(el);
    const visibleFocus =
      (style.outlineStyle !== "none" && parseFloat(style.outlineWidth) > 0) ||
      style.boxShadow !== "none";
    return { name, tag: el.tagName.toLowerCase(), visibleFocus };
  });
}

/** Presses Tab until the focused element's name matches; checks its focus is visible. */
async function tabTo(page: Page, name: RegExp, max = 80): Promise<void> {
  const seen: string[] = [];
  for (let i = 0; i < max; i++) {
    await page.keyboard.press("Tab");
    const f = await focusedName(page);
    seen.push(f.name);
    if (name.test(f.name)) {
      if (["button", "a", "input", "select", "textarea"].includes(f.tag)) {
        // Let the focus ring transition start before reading the style.
        await expect
          .poll(async () => (await focusedName(page)).visibleFocus, {
            message: `visible focus on "${f.name}"`,
          })
          .toBe(true);
      }
      return;
    }
  }
  throw new Error(`Tab never reached ${name}; focused: ${seen.slice(-12).join(" | ")}`);
}

test("a public booking can be completed with the keyboard only", async ({ page }) => {
  test.setTimeout(150_000);
  const target = bookableDay(10);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByRole("button", { name: /Date del soggiorno/ })).toBeVisible();

  // 1. Quando: open the calendar, move a month at a time with PageDown, then
  // by day with the arrows; Enter twice on the same day = a single day.
  await tabTo(page, /Date del soggiorno/);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Date del soggiorno" })).toBeVisible();
  // Into the grid: the first enabled day takes the focus.
  for (let i = 0; i < 10; i++) {
    const inGrid = await page.evaluate(() => !!document.activeElement?.closest('[role="grid"]'));
    if (inGrid) break;
    await page.keyboard.press("Tab");
  }
  for (let i = 0; i < 14; i++) {
    if (target.month.test((await focusedName(page)).name)) break;
    await page.keyboard.press("PageDown");
  }
  for (let i = 0; i < 40; i++) {
    const n = (await focusedName(page)).name;
    if (target.name.test(n)) break;
    const current = Number(/\b(\d{1,2}) \p{L}+ \d{4}/u.exec(n)?.[1] ?? 0);
    await page.keyboard.press(current < target.day ? "ArrowRight" : "ArrowLeft");
  }
  expect((await focusedName(page)).name).toMatch(target.name);
  await page.keyboard.press("Enter");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Date del soggiorno" })).toBeHidden();
  // Focus returns to the field, which now shows the day.
  await expect(page.getByRole("button", { name: /Date del soggiorno/ })).toBeFocused();
  await expect(page.getByRole("button", { name: /Date del soggiorno/ })).toHaveText(
    new RegExp(`^${target.display}$`),
  );
  await tabTo(page, /^Avanti$/);
  await page.keyboard.press("Enter");

  // 2. Dove: the step heading takes the focus; list view, first free umbrella.
  await expect(page.getByRole("heading", { name: "2. Dove" })).toBeFocused();
  await expect(page.getByRole("button", { name: "Mostra come elenco" })).toBeVisible();
  await tabTo(page, /^Mostra come elenco$/);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("region", { name: "Elenco delle postazioni" })).toBeVisible();
  // Gazebos (GZ*) only sell the "Gazebo" package (no "Base" below): umbrellas only.
  await tabTo(page, /^(?!.*(?:Gazebo|GZ))\S+ Libero/, 200);
  await page.keyboard.press("Enter");
  const chosen = page.getByRole("button", { name: /^\S+ Libero/, pressed: true });
  await expect(chosen).toBeFocused();
  // Roving tabindex per row: arrows move within the row and back.
  await page.keyboard.press("End");
  await page.keyboard.press("Home");
  for (let i = 0; i < 40; i++) {
    if (await chosen.evaluate((el) => el === document.activeElement)) break;
    await page.keyboard.press("ArrowRight");
  }
  await expect(chosen).toBeFocused();
  // One tab stop per row: "Avanti" is a handful of Tabs away, not one per spot.
  await tabTo(page, /^Avanti$/, 60);
  await page.keyboard.press("Enter");

  // 3. Cosa: the packages are a radio group (one tab stop, arrows select).
  await expect(page.getByRole("heading", { name: "3. Cosa" })).toBeFocused();
  await expect(page.getByRole("radio", { name: /^Base/ })).toBeVisible();
  await tabTo(page, /./);
  for (let i = 0; i < 6; i++) {
    const base = page.getByRole("radio", { name: /^Base/ });
    if ((await base.getAttribute("aria-checked")) === "true") break;
    await page.keyboard.press("ArrowDown");
  }
  await expect(page.getByRole("radio", { name: /^Base/ })).toHaveAttribute("aria-checked", "true");
  // "Continua" is disabled (out of the tab order) while the price updates.
  await expect(page.getByRole("button", { name: "Continua" })).toBeEnabled();
  await tabTo(page, /^Continua$/);
  await page.keyboard.press("Enter");

  // 4. Chi e pagamento: type the details, Space ticks the terms, Enter confirms.
  await expect(page.getByRole("heading", { name: "4. Chi e pagamento" })).toBeFocused();
  await expect(page.getByRole("textbox", { name: "Nome", exact: true })).toBeVisible();
  await tabTo(page, /^Nome$/);
  await page.keyboard.type("Marco");
  await tabTo(page, /^Cognome$/);
  await page.keyboard.type("Tastiera");
  await tabTo(page, /^Email$/);
  await page.keyboard.type(`marco.tastiera.${Date.now()}@mailbox.test`);
  await tabTo(page, /accetto le condizioni/);
  await page.keyboard.press("Space");
  await expect(page.getByRole("checkbox", { name: /accetto le condizioni/ })).toBeChecked();
  await tabTo(page, /^(Paga|Conferma|Prenota|Invia)/);
  await page.keyboard.press("Enter");

  await expect(page.getByRole("heading", { name: "Prenotazione confermata" })).toBeFocused({
    timeout: 30_000,
  });
  await expect(page.getByTestId("booking-code")).toHaveText(/^LD-\d{4}-\d{6}$/);
});

test.describe("panel", () => {
  test("the main daily pages and the configuration forms have no serious violations", async ({
    page,
  }) => {
    test.setTimeout(180_000);
    await page.goto("/admin/login");
    await expect(page.getByLabel("Email")).toBeVisible();
    expect(await seriousViolations(page), "login").toEqual([]);

    await login(page);
    const settingsLink = page
      .getByRole("navigation", { name: "Configurazione dello stabilimento" })
      .getByRole("link", { name: "Stabilimento", exact: true });
    const id = /\/e\/([0-9a-f-]{36})/.exec((await settingsLink.getAttribute("href")) ?? "")?.[1];
    expect(id, "establishment id from the sidebar").toBeTruthy();

    const pages: [string, string][] = [
      ["oggi", "Oggi"],
      ["mappa", "Mappa"],
      ["prenotazioni", "Prenotazioni"],
      ["cassa", "Cassa"],
      ["vendita-rapida", "Vendita rapida"],
      ["ticket", "Ticket"],
      ["ticket/varco", "Varco"],
      ["config/online", "Pagina pubblica"],
      ["config/payments", "Pagamenti"],
      ["config/email", "Email"],
      ["config/customer-emails", "Email ai clienti"],
    ];
    for (const [path, label] of pages) {
      await page.goto(`/admin/e/${id}/${path}`);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await page.waitForLoadState("networkidle");
      // The email preview is a sandboxed srcdoc frame axe cannot enter (it
      // waits for it until the timeout): the page around it is checked.
      const exclude = path === "config/customer-emails" ? "iframe" : undefined;
      expect(await seriousViolations(page, { exclude }), `${label} (${path})`).toEqual([]);
    }

    // A booking's detail page, from the list.
    await page.goto(`/admin/e/${id}/prenotazioni`);
    const row = page.getByRole("row", { name: /^LD-/ }).first();
    await expect(row).toBeVisible();
    // Rows open with the keyboard too (focusable, Enter).
    await row.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("heading", { level: 1 })).toContainText(/LD-/);
    await page.waitForLoadState("networkidle");
    expect(await seriousViolations(page), "booking detail").toEqual([]);
  });
});
