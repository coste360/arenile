import { readFileSync } from "node:fs";
import { runFile } from "./env";

/** Ids resolved once by data.setup.ts and shared with every capture. */
export interface RunContext {
  est: string;
  slug: string;
  establishmentName: string;
  /** Demo operating date (YYYY-MM-DD) shown by the day views. */
  date: string;
  seasonId: string;
  mapId: string;
  bookingId: string;
  customerId: string;
  /** Path of a fresh magic link of bookingId ("" until the endpoint is implemented). */
  manageUrl: string;
}

export function loadContext(): RunContext {
  return JSON.parse(readFileSync(runFile("context.json"), "utf8")) as RunContext;
}

/** Placeholders of `route` that have no value in this run (feature not available yet). */
export function missingPlaceholders(route: string, ctx: RunContext): string[] {
  return [...route.matchAll(/\{([a-zA-Z]+)\}/g)]
    .map((m) => m[1])
    .filter((key) => !(ctx as unknown as Record<string, string>)[key]);
}

/** Expands the {placeholders} of a manifest route. */
export function expandRoute(route: string, ctx: RunContext): string {
  return route.replace(/\{([a-zA-Z]+)\}/g, (_m, key: string) => {
    const value = (ctx as unknown as Record<string, string>)[key];
    if (!value) throw new Error(`route ${route}: no value for {${key}}`);
    return value;
  });
}
