import type { APIRequestContext } from '@playwright/test';

export const DEMO_SLUG = 'lido-demo';

/** Reads the demo establishment from the public API (name is owned by `seed demo`). */
export async function demoEstablishment(request: APIRequestContext): Promise<{ name: string; slug: string }> {
  const res = await request.get(`/api/public/v1/${DEMO_SLUG}`);
  if (!res.ok()) throw new Error(`GET /api/public/v1/${DEMO_SLUG} -> ${res.status()}`);
  return (await res.json()) as { name: string; slug: string };
}
