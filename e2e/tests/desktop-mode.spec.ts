import { expect, test, type Page } from "@playwright/test";
import { DEMO_SLUG, login } from "./helpers";

// Spec 29.1 "Limiti voluti": on the desktop edition the public page and the
// online payments are replaced by an explanation of how to move to an online
// instance, and pairing waits for the LAN mode. The e2e server is a server
// edition, so the desktop metadata is simulated by intercepting /meta.

type Mode = "local" | "lan";

async function simulateDesktop(page: Page, mode: Mode) {
  await page.route("**/api/admin/v1/meta", async (route) => {
    const res = await route.fetch();
    const meta = (await res.json()) as Record<string, unknown>;
    const desktop =
      mode === "lan"
        ? {
            mode,
            lan_url: "https://192.168.1.20:8443",
            lan_cert_fingerprint: Array.from({ length: 32 }, () => "AB").join(
              ":",
            ),
          }
        : { mode };
    await route.fulfill({
      response: res,
      json: { ...meta, edition: "desktop", desktop },
    });
  });
}

async function establishmentId(page: Page): Promise<string> {
  const res = await page.request.get("/api/admin/v1/establishments");
  const items = (
    (await res.json()) as { items: { id: string; slug: string }[] }
  ).items;
  const id = items.find((e) => e.slug === DEMO_SLUG)?.id;
  expect(id).toBeTruthy();
  return id as string;
}

for (const viewport of [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
]) {
  test.describe(`desktop edition at ${viewport.width}px`, () => {
    test.use({ viewport });

    test("online payments and public page are explained, not offered", async ({
      page,
    }) => {
      await simulateDesktop(page, "local");
      await login(page);
      const id = await establishmentId(page);

      await page.goto(`/admin/e/${id}/config/payments`);
      await expect(
        page.getByRole("heading", { name: "Pagamenti online" }),
      ).toBeVisible();
      await expect(page.getByText("Incassi solo dalla cassa")).toBeVisible();
      await expect(
        page.getByText("arenile restore --in arenile-backup.db"),
      ).toBeVisible();

      await page.goto(`/admin/e/${id}/config/online`);
      await expect(
        page.getByText("Pagina pubblica non disponibile"),
      ).toBeVisible();
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow).toBeLessThanOrEqual(0);
      // The shadcn AlertTitle is one line (line-clamp-1): the notice must fit.
      const clipped = await page
        .locator('[data-slot="alert-title"]')
        .evaluateAll(
          (els) => els.filter((el) => el.scrollWidth > el.clientWidth).length,
        );
      expect(clipped).toBe(0);
    });

    test("pairing waits for the LAN mode in local mode", async ({ page }) => {
      await simulateDesktop(page, "local");
      await login(page);
      const id = await establishmentId(page);
      await page.goto(`/admin/e/${id}/config/devices`);
      await expect(
        page.getByText("Attiva la modalità rete locale nell'app desktop", {
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Abbina un dispositivo" }),
      ).toBeDisabled();
    });

    test("LAN page shows the address and the certificate fingerprint", async ({
      page,
    }) => {
      await simulateDesktop(page, "lan");
      await login(page);
      const id = await establishmentId(page);
      await page.goto(`/admin/e/${id}/config/devices/lan`);
      await expect(
        page.getByRole("heading", { name: "Rete locale" }),
      ).toBeVisible();
      await expect(
        page.getByText("https://192.168.1.20:8443").first(),
      ).toBeVisible();
      const fingerprint = page.getByRole("group", {
        name: "Impronta SHA-256 del certificato",
      });
      await expect(fingerprint).toBeVisible();
      expect((await fingerprint.innerText()).replace(/\s+/g, ":")).toBe(
        Array.from({ length: 32 }, () => "AB").join(":"),
      );
    });
  });
}
