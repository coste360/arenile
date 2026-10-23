/**
 * Checkout session token store (spec 12.2): the token returned by
 * `createPublicBooking` lives in `sessionStorage` (per tab, survives the
 * redirect to the payment provider and back) with an in-memory fallback
 * when storage is unavailable (Safari private mode, sandboxed iframes,
 * blocked third-party storage). Never a cookie, never in the URL, never
 * logged. Sent as the `X-Arenile-Checkout` header.
 */
export const CHECKOUT_HEADER = 'X-Arenile-Checkout';

const PREFIX = 'arenile:checkout:';

export interface StoredCheckout {
  bookingId: string;
  token: string;
  /** RFC 3339 hold expiry, for the countdown after a reload. */
  holdExpiresAt?: string;
}

export interface CheckoutStore {
  load(slug: string): StoredCheckout | null;
  save(slug: string, value: StoredCheckout): void;
  clear(slug: string): void;
}

function isStored(value: unknown): value is StoredCheckout {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.bookingId === 'string' && typeof v.token === 'string';
}

/** Builds a store on `storage`, falling back to memory when it throws. */
export function createCheckoutStore(storage?: Storage | null): CheckoutStore {
  const memory = new Map<string, string>();
  const read = (key: string): string | null => {
    try {
      if (storage) return storage.getItem(key);
    } catch {
      // storage blocked: fall through to memory
    }
    return memory.get(key) ?? null;
  };
  const write = (key: string, value: string | null) => {
    if (value === null) memory.delete(key);
    else memory.set(key, value);
    try {
      if (!storage) return;
      if (value === null) storage.removeItem(key);
      else storage.setItem(key, value);
    } catch {
      // memory already updated
    }
  };
  return {
    load(slug) {
      const raw = read(PREFIX + slug);
      if (!raw) return null;
      try {
        const parsed: unknown = JSON.parse(raw);
        return isStored(parsed) ? parsed : null;
      } catch {
        return null;
      }
    },
    save(slug, value) {
      write(PREFIX + slug, JSON.stringify(value));
    },
    clear(slug) {
      write(PREFIX + slug, null);
    },
  };
}

function defaultStorage(): Storage | null {
  try {
    return typeof window !== 'undefined' ? window.sessionStorage : null;
  } catch {
    return null;
  }
}

/** Store used by the app (sessionStorage of the current tab). */
export const checkoutStore: CheckoutStore = createCheckoutStore(defaultStorage());
