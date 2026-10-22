import { changeLocale, isLocale, type Locale } from '@arenile/shared/i18n';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { GlobeIcon } from 'lucide-react';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { languageTriggerClass } from './languageTrigger';

/**
 * Language picker of the public header (shadcn Select). Lazy-loaded by
 * BookingPage: Radix Select and its positioning engine stay out of the
 * initial bundle (spec 12.6, 150 KB budget).
 */
export default function LanguageSelect({ languages }: { languages: readonly Locale[] }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  return (
    <Select
      value={i18n.language}
      onValueChange={(v) => {
        if (isLocale(v)) void changeLocale(i18n, v);
      }}
    >
      <span id={`${id}-label`} className="sr-only">
        {t('common.language')}
      </span>
      <SelectTrigger
        size="sm"
        aria-labelledby={`${id}-label ${id}-value`}
        className={languageTriggerClass}
      >
        <GlobeIcon aria-hidden="true" />
        <SelectValue id={`${id}-value`} />
      </SelectTrigger>
      <SelectContent align="end">
        {languages.map((l) => (
          <SelectItem key={l} value={l}>
            {t(`common.languages.${l}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
