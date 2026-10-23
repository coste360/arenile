import { useEffect } from 'react';

/** Product name appended to every panel title ("Oggi · Lido Demo · Arenile"). */
export const TITLE_SUFFIX = 'Arenile';

/** Joins the non-empty parts, most specific first, and appends the product name. */
export function documentTitle(...parts: (string | null | undefined)[]): string {
  return [...parts.filter((p): p is string => !!p), TITLE_SUFFIX].join(' · ');
}

/** Sets document.title while mounted (restores the previous title on unmount). */
export function useDocumentTitle(...parts: (string | null | undefined)[]): void {
  const title = documentTitle(...parts);
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}
