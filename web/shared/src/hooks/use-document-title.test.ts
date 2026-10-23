import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { documentTitle, useDocumentTitle } from './use-document-title';

describe('documentTitle', () => {
  it('joins the parts and appends the product name', () => {
    expect(documentTitle('Oggi', 'Lido Demo')).toBe('Oggi · Lido Demo · Arenile');
    expect(documentTitle(undefined, '', null)).toBe('Arenile');
  });

  it('sets and restores document.title', () => {
    document.title = 'before';
    const { unmount } = renderHook(() => useDocumentTitle('Accedi'));
    expect(document.title).toBe('Accedi · Arenile');
    unmount();
    expect(document.title).toBe('before');
  });
});
