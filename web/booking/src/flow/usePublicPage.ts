import { usePublicEstablishment } from '@arenile/shared/api';
import { resolveLocale } from '@arenile/shared/i18n';
import { changePublicLocale } from '../i18n';
import { applyTheme, resolveTheme, type Theme } from '@arenile/shared/theme';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { connectParent, observeHeight } from '../embed/parent';
import { listenPreviewTheme } from '../embed/preview';
import type { BookingLocation } from '../location';
import { useColorScheme } from './colorScheme';
import { DEFAULT_ONLINE, todayIn, type BookingEnv } from './context';

/**
 * Shared shell of the public views (booking flow and manage area): loads
 * the establishment, applies its theme (or the panel preview draft) as the
 * shadcn CSS variables with the computed WCAG AA foregrounds, the colour
 * scheme, the verified embed channel with height reporting, and builds the
 * BookingEnv of the steps.
 */
export function usePublicPage(location: BookingLocation, win: Window, now: () => number) {
  const query = usePublicEstablishment(location.slug);
  const establishment = query.data;
  const [previewTheme, setPreviewTheme] = useState<Theme | null>(null);
  const theme = useMemo(
    () => resolveTheme(previewTheme ?? establishment?.theme),
    [previewTheme, establishment?.theme],
  );
  const channel = useMemo(
    () => (location.embed ? connectParent(win, location.parentOrigin) : null),
    [location.embed, location.parentOrigin, win],
  );

  const { i18n } = useTranslation();

  useColorScheme(win, location.colorScheme);

  // Language: ?lang, then browser, then establishment default (spec 20).
  useEffect(() => {
    if (!establishment) return;
    const desired = resolveLocale({
      param: location.lang,
      browser: win.navigator.languages,
      establishmentDefault: establishment.default_locale,
      enabled: establishment.enabled_locales,
    });
    if (desired !== i18n.language) void changePublicLocale(i18n, desired);
  }, [establishment, location.lang, i18n, win]);

  useEffect(() => {
    if (!location.preview) return;
    return listenPreviewTheme(win, setPreviewTheme);
  }, [location.preview, win]);

  useEffect(() => {
    applyTheme(theme, win.document.documentElement);
  }, [theme, win]);

  useEffect(() => {
    if (!channel) return;
    return observeHeight(win, channel);
  }, [channel, win]);

  const env = useMemo<BookingEnv | null>(
    () =>
      establishment && location.slug
        ? {
            slug: location.slug,
            establishment,
            online: establishment.online ?? DEFAULT_ONLINE,
            theme,
            location,
            channel,
            win,
            preview: location.preview,
            today: todayIn(establishment.timezone, new Date(now())),
          }
        : null,
    [establishment, location, theme, channel, win, now],
  );

  return { query, establishment, theme, channel, env };
}
