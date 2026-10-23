/**
 * Deletes the Cache Storage entries that are not the service worker
 * precache (logout, spec 22/23). The precache holds only the app shell and
 * static assets, never API data, and is kept so the panel still starts
 * offline; anything else (a runtime cache added later by mistake, an old
 * build's cache) is removed. Resolves to the names deleted.
 */
export async function clearRuntimeCaches(
  keep: (name: string) => boolean = (name) => name.includes('precache'),
): Promise<string[]> {
  if (typeof caches === 'undefined') return [];
  const names = await caches.keys();
  const doomed = names.filter((n) => !keep(n));
  await Promise.all(doomed.map((n) => caches.delete(n)));
  return doomed;
}
