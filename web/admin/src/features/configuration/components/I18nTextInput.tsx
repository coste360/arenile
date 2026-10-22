import { Input, Label } from '@arenile/shared/ui';
import type { FieldValues, Path, UseFormRegister } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { errorAt, fieldAria } from '../lib/aria';
import { FieldError, Textarea } from './fields';

export interface I18nTextInputProps<T extends FieldValues> {
  /** Form path of the I18nText (record locale → text). */
  name: string;
  label: string;
  locales: readonly string[];
  defaultLocale: string;
  register: UseFormRegister<T>;
  errors: unknown;
  disabled?: boolean;
  multiline?: boolean;
  idPrefix: string;
}

/**
 * One input per enabled language; the default language comes first and is
 * mandatory, the others fall back to it when empty (spec 20).
 */
export function I18nTextInput<T extends FieldValues>({
  name,
  label,
  locales,
  defaultLocale,
  register,
  errors,
  disabled,
  multiline,
  idPrefix,
}: I18nTextInputProps<T>) {
  const { t } = useTranslation('configuration');
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-slate-800">{label}</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {locales.map((locale) => {
          const path = `${name}.${locale}`;
          const id = `${idPrefix}-${name}-${locale}`.replace(/\./g, '-');
          const error = errorAt(errors, path);
          const lang = t(`common.languages.${locale}`, { ns: 'translation', defaultValue: locale });
          const fieldLabel =
            locale === defaultLocale ? t('i18nText.default', { language: lang }) : lang;
          const props = {
            ...fieldAria(id, error),
            lang: locale,
            disabled,
            placeholder: locale === defaultLocale ? undefined : t('i18nText.fallbackPlaceholder'),
            ...register(path as Path<T>),
          };
          return (
            <div key={locale} className="space-y-1">
              <Label htmlFor={id} className="text-xs font-normal text-slate-600">
                {fieldLabel}
              </Label>
              {multiline ? <Textarea {...props} /> : <Input {...props} />}
              <FieldError id={id} error={error} />
            </div>
          );
        })}
      </div>
    </fieldset>
  );
}
