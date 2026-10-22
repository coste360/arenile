import { changeLocale, isLocale, type Locale } from '@arenile/shared/i18n';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { GlobeIcon } from 'lucide-react';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Language picker of the public header: the shadcn NativeSelect (a styled
 * <select>), so no Radix positioning code lands in the first-paint bundle
 * (spec 12.6, 150 KB budget) and the picker works like the device's own.
 */
export function LanguageSelect({ languages }: { languages: readonly Locale[] }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  return (
    <div className="flex items-center gap-1.5 text-primary-foreground">
      <GlobeIcon aria-hidden="true" className="size-4 opacity-80" />
      <label htmlFor={id} className="sr-only">
        {t('common.language')}
      </label>
      <NativeSelect
        id={id}
        size="sm"
        value={i18n.language}
        onChange={(e) => {
          if (isLocale(e.target.value)) void changeLocale(i18n, e.target.value);
        }}
        className="border-primary-foreground/40 bg-transparent text-primary-foreground dark:bg-transparent [&>option]:text-foreground"
      >
        {languages.map((l) => (
          <NativeSelectOption key={l} value={l}>
            {t(`common.languages.${l}`)}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </div>
  );
}
