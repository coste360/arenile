import type { Schemas } from '@arenile/shared/api';
import { cn } from '@arenile/shared/lib/utils';
import { Button } from '@arenile/shared/ui/button';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { ExternalLinkIcon, MonitorIcon, MoonIcon, SmartphoneIcon, SunIcon } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { previewUrl, type Scheme } from '../lib/preview';

export interface PublicPagePreviewProps {
  slug: string;
  /** Theme being edited (unsaved). */
  theme: Schemas['Theme'];
  lang: string;
}

type Device = 'mobile' | 'desktop';

/**
 * Live preview of the public page (spec 12.5: "l'anteprima nel pannello è
 * la pagina reale in modalità prova"): the real booking page in an iframe on
 * the same origin, in preview mode (it never books). The unsaved theme is
 * posted to it on every change and when it announces itself; messages are
 * exchanged with our own origin only.
 */
export function PublicPagePreview({ slug, theme, lang }: PublicPagePreviewProps) {
  const { t } = useTranslation('online');
  const ref = useRef<HTMLIFrameElement>(null);
  const [device, setDevice] = useState<Device>('mobile');
  const [scheme, setScheme] = useState<Scheme>('light');
  const origin = window.location.origin;

  const post = useCallback(() => {
    ref.current?.contentWindow?.postMessage({ type: 'arenile:preview-theme', theme }, origin);
  }, [theme, origin]);

  useEffect(() => {
    post();
  }, [post]);

  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      if (e.origin !== origin || e.source !== ref.current?.contentWindow) return;
      if ((e.data as { type?: unknown } | null)?.type === 'arenile:preview-ready') post();
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [origin, post]);

  const src = previewUrl(slug, lang, scheme);
  return (
    <figure className="space-y-3 lg:sticky lg:top-4" aria-labelledby="preview-caption">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <figcaption id="preview-caption" className="text-sm font-medium">
          {t('settings.preview')}
        </figcaption>
        <div className="flex items-center gap-2">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={device}
            onValueChange={(v) => v && setDevice(v as Device)}
            aria-label={t('settings.previewDevice')}
          >
            <ToggleGroupItem value="mobile" aria-label={t('settings.previewMobile')}>
              <SmartphoneIcon aria-hidden="true" />
            </ToggleGroupItem>
            <ToggleGroupItem value="desktop" aria-label={t('settings.previewDesktop')}>
              <MonitorIcon aria-hidden="true" />
            </ToggleGroupItem>
          </ToggleGroup>
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={scheme}
            onValueChange={(v) => v && setScheme(v as Scheme)}
            aria-label={t('settings.previewScheme')}
          >
            <ToggleGroupItem value="light" aria-label={t('settings.previewLight')}>
              <SunIcon aria-hidden="true" />
            </ToggleGroupItem>
            <ToggleGroupItem value="dark" aria-label={t('settings.previewDark')}>
              <MoonIcon aria-hidden="true" />
            </ToggleGroupItem>
          </ToggleGroup>
          <Button asChild variant="ghost" size="icon-sm">
            <a href={src} target="_blank" rel="noreferrer" aria-label={t('settings.previewOpen')}>
              <ExternalLinkIcon aria-hidden="true" />
            </a>
          </Button>
        </div>
      </div>
      <div className="flex justify-center overflow-hidden rounded-xl border bg-muted/40 p-2">
        <iframe
          ref={ref}
          src={src}
          title={t('settings.previewFrame')}
          onLoad={post}
          className={cn(
            'h-[40rem] max-w-full rounded-lg border bg-background shadow-sm transition-[width]',
            device === 'mobile' ? 'w-[24rem]' : 'w-full',
          )}
        />
      </div>
      <p className="text-xs text-muted-foreground">{t('settings.previewHint')}</p>
    </figure>
  );
}
