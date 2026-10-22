import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@arenile/shared/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import { Textarea } from '@arenile/shared/ui/textarea';
import { cn } from '@arenile/shared/lib/utils';

export const I18N_TEXT_LOCALES = ['it', 'en', 'de', 'fr'] as const;

/** Translated text as stored by the API: locale → text. */
export type I18nText = Partial<Record<string, string>>;

export interface I18nTextInputProps {
  /** Base id; each locale gets `${id}-${locale}`. The default locale's input gets `id` itself. */
  id?: string;
  value: I18nText | null | undefined;
  onChange: (value: I18nText) => void;
  /** Enabled locales, default locale first is not required (default it/en/de/fr). */
  locales?: readonly string[];
  /** Mandatory locale, shown first (default "it"). */
  defaultLocale?: string;
  multiline?: boolean;
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  'aria-invalid'?: boolean | 'true' | 'false';
  'aria-describedby'?: string;
  'aria-labelledby'?: string;
}

/**
 * Text in several languages with one tab per locale (spec 20). The default
 * locale comes first; other tabs show a "da tradurre" dot while empty and
 * fall back to the default text on the public pages.
 */
export function I18nTextInput({
  id,
  value,
  onChange,
  locales = I18N_TEXT_LOCALES,
  defaultLocale = 'it',
  multiline,
  disabled,
  placeholder,
  className,
  ...aria
}: I18nTextInputProps) {
  const { t } = useTranslation();
  const autoId = useId();
  const baseId = id ?? autoId;
  const ordered = [defaultLocale, ...locales.filter((l) => l !== defaultLocale)];
  const [tab, setTab] = useState(defaultLocale);
  const text = value ?? {};
  const Control = multiline ? Textarea : Input;

  return (
    <Tabs value={tab} onValueChange={setTab} className={cn('gap-2', className)}>
      <TabsList aria-label={t('ui.i18nText.languages')}>
        {ordered.map((locale) => {
          const missing = locale !== defaultLocale && !text[locale]?.trim();
          return (
            <TabsTrigger key={locale} value={locale} className="gap-1.5 uppercase">
              {locale}
              {missing && (
                <>
                  <span aria-hidden="true" className="size-1.5 rounded-full bg-highlight" />
                  <span className="sr-only">({t('ui.i18nText.missing')})</span>
                </>
              )}
            </TabsTrigger>
          );
        })}
      </TabsList>
      {ordered.map((locale) => (
        <TabsContent key={locale} value={locale} forceMount hidden={tab !== locale}>
          <Control
            id={locale === defaultLocale ? baseId : `${baseId}-${locale}`}
            lang={locale}
            disabled={disabled}
            value={text[locale] ?? ''}
            placeholder={
              locale === defaultLocale ? placeholder : (text[defaultLocale] ?? placeholder)
            }
            onChange={(e) => onChange({ ...text, [locale]: e.target.value })}
            {...aria}
            aria-label={
              locale === defaultLocale
                ? undefined
                : t(`common.languages.${locale}`, { defaultValue: locale })
            }
          />
        </TabsContent>
      ))}
    </Tabs>
  );
}
