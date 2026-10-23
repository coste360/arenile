import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/** Paths and fixed values of the documentation capture run. */
const here = dirname(fileURLToPath(import.meta.url));

export const REPO_ROOT = resolve(here, "..", "..", "..");
export const DOCS_ROOT = join(REPO_ROOT, "docs-site");
export const MANIFEST_PATH = join(DOCS_ROOT, "screenshots.manifest.json");
export const SCREENSHOTS_DIR = join(DOCS_ROOT, "src", "assets", "screenshots");
export const DATA_DIR = join(DOCS_ROOT, "src", "data");

/** Demo establishment and owner created by `arenile seed demo`. */
export const DEMO_SLUG = "lido-demo";
export const DEMO_OWNER_EMAIL = "demo@lido-demo.example";
/** Password given to the demo owner through ARENILE_ADMIN_PASSWORD (docs-only value). */
export const DOCS_OWNER_PASSWORD = process.env.DOCS_OWNER_PASSWORD ?? "Docs-Demo-Owner-2026!";

/** Run-scoped files shared by the `data` and `capture` projects. */
export function runFile(name: string): string {
  const dir = process.env.ARENILE_DOCS_DATA_DIR;
  if (!dir) throw new Error("ARENILE_DOCS_DATA_DIR is not set (run through playwright.config.ts)");
  return join(dir, name);
}
