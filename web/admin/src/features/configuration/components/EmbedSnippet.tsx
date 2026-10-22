import { Button } from '@arenile/shared/ui';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { embedSnippets } from '../lib/publicPage';

function CopyBox({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('configuration');
  const id = useId();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard denied: the text stays selectable in the box.
      const el = document.getElementById(id) as HTMLTextAreaElement | null;
      el?.select();
    }
  };
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <label htmlFor={id} className="text-sm font-medium text-slate-800">
          {label}
        </label>
        <Button size="sm" variant="outline" onClick={() => void copy()}>
          {copied ? t('embed.copied') : t('embed.copy')}
        </Button>
      </div>
      <textarea
        id={id}
        readOnly
        value={value}
        rows={value.split('\n').length}
        onFocus={(e) => e.currentTarget.select()}
        className="w-full resize-none rounded-md border border-slate-300 bg-slate-50 p-2 font-mono text-xs text-slate-800"
      />
    </div>
  );
}

/** Public page link and the two embed snippets of spec 12.1 with copy buttons. */
export function EmbedSnippet({ slug, lang }: { slug: string; lang: string }) {
  const { t } = useTranslation('configuration');
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const s = embedSnippets(origin, slug, lang);
  return (
    <div className="space-y-4">
      <p className="text-sm">
        {t('embed.publicUrl')}{' '}
        <a
          href={s.url}
          target="_blank"
          rel="noreferrer"
          className="break-all text-[var(--ar-primary-on-surface)] underline underline-offset-2"
        >
          {s.url}
        </a>
      </p>
      <CopyBox label={t('embed.script')} value={s.script} />
      <CopyBox label={t('embed.iframe')} value={s.iframe} />
      <p className="text-xs text-slate-600">{t('embed.hint')}</p>
    </div>
  );
}
