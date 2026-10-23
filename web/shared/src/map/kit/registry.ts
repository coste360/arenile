/**
 * Which kit symbols the page needs. Every <MapKitDefs ids> instance (one per
 * map, palette or thumbnail grid) registers the ids it draws; the first
 * mounted instance is the leader and renders the ONE hidden sprite with the
 * union, so ids are never duplicated in the document however many maps are
 * on screen. When the leader unmounts, the next instance takes over.
 */
type Listener = () => void;

const requests = new Map<symbol, ReadonlySet<string>>();
const listeners = new Set<Listener>();

/** Immutable view of the registry, replaced on every change (useSyncExternalStore snapshot). */
export interface KitIdsSnapshot {
  /** The instance that renders the sprite (first registered still mounted). */
  leader: symbol | undefined;
  /** Sorted union of every registered id. */
  ids: readonly string[];
}

let snapshot: KitIdsSnapshot = { leader: undefined, ids: [] };

function emit() {
  const all = new Set<string>();
  for (const ids of requests.values()) for (const id of ids) all.add(id);
  snapshot = { leader: requests.keys().next().value, ids: [...all].sort() };
  for (const l of listeners) l();
}

export function registerKitIds(token: symbol, ids: ReadonlySet<string>): void {
  const prev = requests.get(token);
  if (prev && prev.size === ids.size && [...ids].every((id) => prev.has(id))) return;
  requests.set(token, ids);
  emit();
}

export function unregisterKitIds(token: symbol): void {
  if (requests.delete(token)) emit();
}

export function subscribeKitIds(l: Listener): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function kitIdsSnapshot(): KitIdsSnapshot {
  return snapshot;
}
