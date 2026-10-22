import { applyTheme, resolveTheme, type Theme } from '@arenile/shared/theme';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { cn } from '@arenile/shared/lib/utils';
import { TriangleAlertIcon, UmbrellaIcon } from 'lucide-react';
import { useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { themeWarnings } from '../lib/publicPage';

export interface ThemePreviewProps {
  theme: Theme;
  name: string;
  /** Intro text in the preview language (custom_texts.<lang>.intro). */
  intro?: string;
}

/**
 * Live preview of the public page theme (spec 12.5): the same shadcn CSS
 * variables the booking page writes with applyTheme(), scoped to a miniature
 * of its header, map and summary built with the same components.
 */
export function ThemePreview({ theme, name, intro }: ThemePreviewProps) {
  const { t } = useTranslation('configuration');
  const resolved = resolveTheme(theme);
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (ref.current) applyTheme(resolved, ref.current);
  });
  const spots = Array.from({ length: 18 }, (_, i) => i);
  return (
    <figure className="space-y-2 lg:sticky lg:top-4">
      <figcaption className="text-sm font-medium">{t('theme.preview')}</figcaption>
      <Card
        ref={ref}
        data-testid="theme-preview"
        className="gap-0 overflow-hidden py-0 font-sans shadow-md"
      >
        <CardHeader className="flex flex-row items-center gap-2 bg-primary px-4 py-3 text-primary-foreground">
          {resolved.logoUrl ? (
            <img src={resolved.logoUrl} alt="" className="h-6 w-auto" />
          ) : (
            <UmbrellaIcon aria-hidden="true" className="size-5 opacity-80" />
          )}
          <CardTitle className="text-base">{name}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4 p-4">
          <p className="text-sm text-muted-foreground">{intro || t('theme.previewIntro')}</p>
          <div className="rounded-lg bg-map-sand/40 p-2" aria-hidden="true">
            <div className="grid grid-cols-6 gap-1.5">
              {spots.map((i) => (
                <span
                  key={i}
                  className={cn(
                    'flex h-7 items-center justify-center rounded-md text-xs font-medium',
                    i === 7
                      ? 'bg-highlight text-highlight-foreground ring-2 ring-primary'
                      : i % 5 === 0
                        ? 'bg-muted text-muted-foreground'
                        : 'border border-primary bg-background text-primary-on-surface',
                  )}
                >
                  {resolved.showPricesOnMap && i % 5 !== 0 ? '€' : ''}
                </span>
              ))}
            </div>
          </div>
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Badge className="bg-highlight text-highlight-foreground">
                {t('theme.previewBadge')}
              </Badge>
              <span className="text-sm text-primary-on-surface underline underline-offset-2">
                {t('theme.previewLink')}
              </span>
            </div>
            <Button size="sm" tabIndex={-1} aria-hidden="true">
              {t('theme.previewButton')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </figure>
  );
}

/** Contrast warnings of the theme being edited. */
export function ThemeWarnings({ theme }: { theme: Theme }) {
  const { t } = useTranslation('configuration');
  const warnings = themeWarnings(theme);
  if (warnings.length === 0) return null;
  return (
    <Alert role="status" className="border-warning/50 bg-warning/10">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertDescription className="text-foreground">
        <ul className="space-y-1">
          {warnings.map((w) => (
            <li key={w.key}>
              {t(`theme.warnings.${w.key}`, {
                ratio: w.ratio.toFixed(2),
                corrected: w.corrected ?? '',
              })}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}
