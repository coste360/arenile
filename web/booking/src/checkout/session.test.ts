import { describe, expect, it } from 'vitest';
import { createCheckoutStore } from './session';

class MemoryStorage implements Storage {
  private data = new Map<string, string>();
  get length() {
    return this.data.size;
  }
  clear() {
    this.data.clear();
  }
  getItem(key: string) {
    return this.data.get(key) ?? null;
  }
  key(index: number) {
    return [...this.data.keys()][index] ?? null;
  }
  removeItem(key: string) {
    this.data.delete(key);
  }
  setItem(key: string, value: string) {
    this.data.set(key, value);
  }
}

class BrokenStorage extends MemoryStorage {
  override getItem(): string | null {
    throw new Error('SecurityError');
  }
  override setItem(): void {
    throw new Error('QuotaExceededError');
  }
}

const value = { bookingId: 'b1', token: 't'.repeat(43), holdExpiresAt: '2027-07-10T09:10:00Z' };

describe('checkout store', () => {
  it('round-trips per slug in sessionStorage', () => {
    const storage = new MemoryStorage();
    const store = createCheckoutStore(storage);
    store.save('lido', value);
    expect(createCheckoutStore(storage).load('lido')).toEqual(value);
    expect(store.load('other')).toBeNull();
    store.clear('lido');
    expect(store.load('lido')).toBeNull();
  });

  it('falls back to memory when storage throws', () => {
    const store = createCheckoutStore(new BrokenStorage());
    store.save('lido', value);
    expect(store.load('lido')).toEqual(value);
  });

  it('ignores corrupt entries', () => {
    const storage = new MemoryStorage();
    storage.setItem('arenile:checkout:lido', '{"token":1}');
    expect(createCheckoutStore(storage).load('lido')).toBeNull();
    storage.setItem('arenile:checkout:lido', 'not json');
    expect(createCheckoutStore(storage).load('lido')).toBeNull();
  });
});
