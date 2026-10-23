import { describe, expect, it } from 'vitest';
import { createManageTokenStore, resolveManageToken } from './token';

const TOKEN = 't'.repeat(43);

describe('manage token store', () => {
  it('keeps the token per slug and survives a blocked storage', () => {
    const blocked = {
      getItem() {
        throw new Error('blocked');
      },
      setItem() {
        throw new Error('blocked');
      },
    } as unknown as Storage;
    const store = createManageTokenStore(blocked);
    expect(store.load('lido')).toBeNull();
    store.save('lido', TOKEN);
    expect(store.load('lido')).toBe(TOKEN);
    expect(store.load('altro')).toBeNull();
  });

  it('resolves the path token first and stores it, then the stored one', () => {
    const win = { sessionStorage: window.sessionStorage } as unknown as Window;
    window.sessionStorage.clear();
    expect(resolveManageToken(win, 'lido', null)).toBeNull();
    expect(resolveManageToken(win, 'lido', TOKEN)).toBe(TOKEN);
    expect(resolveManageToken(win, 'lido', null)).toBe(TOKEN);
    expect(window.sessionStorage.getItem('arenile:manage:lido')).toBe(TOKEN);
    window.sessionStorage.clear();
  });
});
