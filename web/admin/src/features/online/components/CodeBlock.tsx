import { FieldLabel, Field as UiField } from '@arenile/shared/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupText,
  InputGroupTextarea,
} from '@arenile/shared/ui/input-group';
import { CheckIcon, CodeIcon, CopyIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';

/**
 * Read-only value with a copy button (shadcn InputGroup + textarea): embed
 * snippets, webhook URL. When the clipboard is denied the text is selected
 * for a manual copy.
 */
export function CodeBlock({
  label,
  value,
  language = 'HTML',
  rows,
}: {
  label: string;
  value: string;
  language?: string;
  rows?: number;
}) {
  const { t } = useTranslation('online');
  const id = useId();
  const ref = useRef<HTMLTextAreaElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(t('copied'));
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
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
            {language}
          </InputGroupText>
          <InputGroupButton
            size="xs"
            className="ml-auto"
            onClick={() => void copy()}
            aria-describedby={`${id}-label`}
          >
            {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
            {copied ? t('copied') : t('copy')}
          </InputGroupButton>
        </InputGroupAddon>
        <InputGroupTextarea
          id={id}
          ref={ref}
          readOnly
          value={value}
          rows={rows ?? value.split('\n').length}
          spellCheck={false}
          onFocus={(e) => e.currentTarget.select()}
          className="resize-none font-mono text-xs leading-relaxed"
        />
      </InputGroup>
    </UiField>
  );
}
