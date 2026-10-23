import { useEffect } from 'react';

const MEDIA = '(prefers-color-scheme: dark)';

/**
 * Light / dark mode of the public page: the `dark` class of the shadcn theme
 * on <html>, following the visitor's system unless `?theme=light|dark`
 * forces it (host pages with a fixed look, documentation screenshots). The
 * panel's ThemeProvider is not used here: it pulls a dropdown menu into the
 * first-screen bundle.
 */
export function useColorScheme(win: Window, forced: 'light' | 'dark' | null): void {
  useEffect(() => {
    const apply = (dark: boolean) => {
      const root = win.document.documentElement;
      root.classList.toggle('dark', dark);
      root.setAttribute('data-color-scheme', dark ? 'dark' : 'light');
    };
    if (forced) {
      apply(forced === 'dark');
      return;
    }
    if (typeof win.matchMedia !== 'function') {
      apply(false);
      return;
    }
    const mql = win.matchMedia(MEDIA);
    apply(mql.matches);
    const onChange = () => apply(mql.matches);
    mql.addEventListener?.('change', onChange);
    return () => mql.removeEventListener?.('change', onChange);
  }, [win, forced]);
}
