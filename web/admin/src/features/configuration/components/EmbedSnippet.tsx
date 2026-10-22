import { Button } from '@arenile/shared/ui/button';
import { FieldDescription, FieldLabel, Field as UiField } from '@arenile/shared/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupText,
  InputGroupTextarea,
} from '@arenile/shared/ui/input-group';
import { CheckIcon, CodeIcon, CopyIcon, ExternalLinkIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { embedSnippets } from '../lib/publicPage';

/** Read-only code block with a copy button (shadcn InputGroup + textarea). */
function CodeBlock({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('configuration');
  const id = useId();
  const ref = useRef<HTMLTextAreaElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(t('embed.copied'));
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard denied: select the text so the user can copy it by hand.
      ref.current?.select();
    }
  };
  return (
    <UiField className="gap-2">
      <FieldLabel id={`${id}-label`} htmlFor={id}>
        {label}
      </FieldLabel>
      <InputGroup className="bg-muted/40">
        <InputGroupAddon align="block-start" className="border-b">
          <InputGroupText className="font-mono text-xs">
            <CodeIcon aria-hidden="true" />
            HTML
          </InputGroupText>
          <InputGroupButton
            size="xs"
            className="ml-auto"
            onClick={() => void copy()}
            aria-describedby={`${id}-label`}
          >
            {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
            {copied ? t('embed.copied') : t('embed.copy')}
          </InputGroupButton>
        </InputGroupAddon>
        <InputGroupTextarea
          id={id}
          ref={ref}
          readOnly
          value={value}
          rows={value.split('\n').length}
          spellCheck={false}
          onFocus={(e) => e.currentTarget.select()}
          className="resize-none font-mono text-xs leading-relaxed"
        />
      </InputGroup>
    </UiField>
  );
}

/** Public page link and the two embed snippets of spec 12.1 with copy buttons. */
export function EmbedSnippet({ slug, lang }: { slug: string; lang: string }) {
  const { t } = useTranslation('configuration');
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const s = embedSnippets(origin, slug, lang);
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/40 px-3 py-2 text-sm">
        <span className="min-w-0">
          <span className="text-muted-foreground">{t('embed.publicUrl')}</span>{' '}
          <span className="font-mono break-all">{s.url}</span>
        </span>
        <Button asChild variant="outline" size="sm">
          <a href={s.url} target="_blank" rel="noreferrer">
            <ExternalLinkIcon aria-hidden="true" />
            {t('embed.open')}
          </a>
        </Button>
      </div>
      <CodeBlock label={t('embed.script')} value={s.script} />
      <CodeBlock label={t('embed.iframe')} value={s.iframe} />
      <FieldDescription>{t('embed.hint')}</FieldDescription>
    </div>
  );
}
