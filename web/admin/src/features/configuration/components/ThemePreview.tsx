import { resolveTheme, themeToCssVars, type Theme } from '@arenile/shared/theme';
import type { CSSProperties } from 'react';
import { useTranslation } from 'react-i18next';
import { themeWarnings } from '../lib/publicPage';

export interface ThemePreviewProps {
  theme: Theme;
  name: string;
  /** Intro text in the preview language (custom_texts.<lang>.intro). */
  intro?: string;
}

/**
 * Live preview of the public page theme (spec 12.5): the same CSS variables
 * the booking page applies, on a miniature of its header, map and summary.
 */
export function ThemePreview({ theme, name, intro }: ThemePreviewProps) {
  const { t } = useTranslation('configuration');
  const resolved = resolveTheme(theme);
  const style = {
    ...themeToCssVars(resolved),
    fontFamily: resolved.fontFamily,
  } as CSSProperties;
  const spots = Array.from({ length: 12 }, (_, i) => i);
  return (
    <figure className="space-y-2">
      <figcaption className="text-sm font-medium text-slate-800">{t('theme.preview')}</figcaption>
      <div
        data-testid="theme-preview"
        style={style}
        className="overflow-hidden rounded-[var(--ar-radius)] border border-slate-200 bg-white text-slate-900 shadow-sm"
      >
        <div className="flex items-center gap-2 bg-[var(--ar-primary)] px-3 py-2 text-[var(--ar-primary-foreground)]">
          {resolved.logoUrl ? (
            <img src={resolved.logoUrl} alt="" className="h-6 w-auto" />
          ) : (
            <span
              aria-hidden="true"
              className="inline-block h-6 w-6 rounded-full bg-[var(--ar-primary-foreground)] opacity-30"
            />
          )}
          <span className="font-semibold">{name}</span>
        </div>
        <div className="space-y-3 p-3">
          <p className="text-sm">{intro || t('theme.previewIntro')}</p>
          <div className="grid grid-cols-6 gap-1.5" aria-hidden="true">
            {spots.map((i) => (
              <span
                key={i}
                className={`flex h-6 items-center justify-center rounded-[var(--ar-radius)] text-[10px] ${
                  i === 4
                    ? 'bg-[var(--ar-accent)] text-[var(--ar-accent-foreground)]'
                    : i % 5 === 0
                      ? 'bg-slate-300 text-slate-700'
                      : 'border border-[var(--ar-primary)] text-[var(--ar-primary-on-surface)]'
                }`}
              >
                {resolved.showPricesOnMap && i % 5 !== 0 ? '€' : ''}
              </span>
            ))}
          </div>
          <div className="flex items-center justify-between gap-2">
            <a className="text-sm text-[var(--ar-primary-on-surface)] underline" href="#preview">
              {t('theme.previewLink')}
            </a>
            <span className="inline-flex h-8 items-center rounded-[var(--ar-radius)] bg-[var(--ar-primary)] px-3 text-sm font-medium text-[var(--ar-primary-foreground)]">
              {t('theme.previewButton')}
            </span>
          </div>
        </div>
      </div>
    </figure>
  );
}

/** Contrast warnings of the theme being edited. */
export function ThemeWarnings({ theme }: { theme: Theme }) {
  const { t } = useTranslation('configuration');
  const warnings = themeWarnings(theme);
  if (warnings.length === 0) return null;
  return (
    <ul
      role="status"
      className="space-y-1 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900"
    >
      {warnings.map((w) => (
        <li key={w.key}>
          {t(`theme.warnings.${w.key}`, {
            ratio: w.ratio.toFixed(2),
            corrected: w.corrected ?? '',
          })}
        </li>
      ))}
    </ul>
  );
}
