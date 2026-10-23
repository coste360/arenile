import { readFileSync } from "node:fs";
import { MANIFEST_PATH } from "./env";

export type Theme = "light" | "dark";
export const THEMES: readonly Theme[] = ["light", "dark"];

export interface Viewport {
  width: number;
  height: number;
}

export interface ShotEntry {
  name: string;
  section: string;
  route: string;
  viewport: string;
  /** false = no login (public pages, login page). Default true. */
  auth?: boolean;
  description: string;
}

export interface Manifest {
  version: number;
  viewports: Record<string, Viewport>;
  screenshots: ShotEntry[];
}

/** docs-site/screenshots.manifest.json, validated (unique names, known viewports). */
export function loadManifest(): Manifest {
  const m = JSON.parse(readFileSync(MANIFEST_PATH, "utf8")) as Manifest;
  const seen = new Set<string>();
  for (const s of m.screenshots) {
    if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(s.name)) throw new Error(`manifest: bad name "${s.name}"`);
    if (seen.has(s.name)) throw new Error(`manifest: duplicate name "${s.name}"`);
    if (!m.viewports[s.viewport]) throw new Error(`manifest: ${s.name} has unknown viewport "${s.viewport}"`);
    seen.add(s.name);
  }
  return m;
}

/** DOCS_ONLY=admin-map,public-* restricts the run to matching names. */
export function selected(name: string): boolean {
  const only = process.env.DOCS_ONLY?.trim();
  if (!only) return true;
  return only.split(",").some((p) => {
    const glob = p.trim().replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*");
    return new RegExp(`^${glob}$`).test(name);
  });
}
