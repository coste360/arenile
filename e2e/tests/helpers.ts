import type { APIRequestContext } from '@playwright/test';

export const DEMO_SLUG = 'lido-demo';

/** Demo owner created by `seed demo` (password from ARENILE_ADMIN_PASSWORD, set by the config). */
export const DEMO_OWNER_EMAIL = 'demo@lido-demo.example';
export const DEMO_OWNER_PASSWORD = process.env.E2E_OWNER_PASSWORD ?? 'E2e-Demo-Owner-2026!';

/** Reads the demo establishment from the public API (name is owned by `seed demo`). */
export async function demoEstablishment(request: APIRequestContext): Promise<{ name: string; slug: string }> {
  const res = await request.get(`/api/public/v1/${DEMO_SLUG}`);
  if (!res.ok()) throw new Error(`GET /api/public/v1/${DEMO_SLUG} -> ${res.status()}`);
  return (await res.json()) as { name: string; slug: string };
}
