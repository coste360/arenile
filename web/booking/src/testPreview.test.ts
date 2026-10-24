import { describe, expect, it } from 'vitest';
import { resolveTestPreviewToken } from './testPreview';

function memory() {
  const m = new Map<string, string>();
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
  };
}

const TOKEN = 'v1:abcd1234:' + 'A'.repeat(40);

describe('resolveTestPreviewToken', () => {
  it('reads the token from the query and keeps it for the slug', () => {
    const s = memory();
    expect(resolveTestPreviewToken('lido', `?preview=${encodeURIComponent(TOKEN)}`, s)).toBe(TOKEN);
    expect(resolveTestPreviewToken('lido', '?checkout=x', s)).toBe(TOKEN);
    expect(resolveTestPreviewToken('altro', '', s)).toBeNull();
  });

  it('ignores the theme preview flag and malformed values', () => {
    const s = memory();
    expect(resolveTestPreviewToken('lido', '?preview=1', s)).toBeNull();
    expect(resolveTestPreviewToken('lido', '?preview=<script>', s)).toBeNull();
    expect(resolveTestPreviewToken(null, `?preview=${TOKEN}`, s)).toBeNull();
  });
});
