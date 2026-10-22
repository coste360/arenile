import { MonitorIcon, MoonIcon, SunIcon } from 'lucide-react';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';

export type ColorScheme = 'light' | 'dark' | 'system';
export type ResolvedColorScheme = 'light' | 'dark';

export const COLOR_SCHEMES: readonly ColorScheme[] = ['light', 'dark', 'system'];
const STORAGE_KEY = 'arenile-theme';
const MEDIA = '(prefers-color-scheme: dark)';

interface ThemeContextValue {
  /** User choice; "system" follows the operating system. */
  theme: ColorScheme;
  /** Scheme actually applied to <html>. */
  resolvedTheme: ResolvedColorScheme;
  setTheme: (theme: ColorScheme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function readStored(): ColorScheme | null {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    return v === 'light' || v === 'dark' || v === 'system' ? v : null;
  } catch {
    return null;
  }
}

function systemScheme(): ResolvedColorScheme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'light';
  return window.matchMedia(MEDIA).matches ? 'dark' : 'light';
}

export interface ThemeProviderProps {
  children: ReactNode;
  /** Initial choice when nothing is stored (default "system"). */
  defaultTheme?: ColorScheme;
  /** Element receiving the `dark` class (default <html>). */
  target?: HTMLElement;
}

/**
 * Light/dark mode for the panel (a tiny next-themes equivalent, no Next.js):
 * toggles the `dark` class used by the shadcn theme in styles/globals.css and
 * remembers the choice in localStorage.
 */
export function ThemeProvider({ children, defaultTheme = 'system', target }: ThemeProviderProps) {
  const [theme, setThemeState] = useState<ColorScheme>(() => readStored() ?? defaultTheme);
  const [system, setSystem] = useState<ResolvedColorScheme>(systemScheme);

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return;
    const mql = window.matchMedia(MEDIA);
    const onChange = () => setSystem(mql.matches ? 'dark' : 'light');
    mql.addEventListener?.('change', onChange);
    return () => mql.removeEventListener?.('change', onChange);
  }, []);

  const resolvedTheme: ResolvedColorScheme = theme === 'system' ? system : theme;

  useEffect(() => {
    const root = target ?? document.documentElement;
    root.classList.toggle('dark', resolvedTheme === 'dark');
    root.style.setProperty('color-scheme', resolvedTheme);
  }, [resolvedTheme, target]);

  const setTheme = useCallback((next: ColorScheme) => {
    setThemeState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Storage unavailable (private mode): the choice lasts for the session.
    }
  }, []);

  const value = useMemo(
    () => ({ theme, resolvedTheme, setTheme }),
    [theme, resolvedTheme, setTheme],
  );
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

/** Current colour scheme; outside a ThemeProvider it reports a fixed light theme. */
export function useTheme(): ThemeContextValue {
  return useContext(ThemeContext) ?? { theme: 'light', resolvedTheme: 'light', setTheme: () => {} };
}

const ICONS = { light: SunIcon, dark: MoonIcon, system: MonitorIcon } as const;

/** Radio items for an existing DropdownMenu (e.g. the user menu). */
export function ThemeRadioItems() {
  const { t } = useTranslation();
  const { theme, setTheme } = useTheme();
  return (
    <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as ColorScheme)}>
      {COLOR_SCHEMES.map((scheme) => {
        const Icon = ICONS[scheme];
        return (
          <DropdownMenuRadioItem key={scheme} value={scheme}>
            <Icon aria-hidden="true" />
            {t(`ui.theme.${scheme}`)}
          </DropdownMenuRadioItem>
        );
      })}
    </DropdownMenuRadioGroup>
  );
}

/** Icon button opening the light/dark/system choice. */
export function ThemeToggle({ className }: { className?: string }) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className={className} aria-label={t('ui.theme.toggle')}>
          <SunIcon className="scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
          <MoonIcon className="absolute scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>{t('ui.theme.label')}</DropdownMenuLabel>
        <ThemeRadioItems />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
