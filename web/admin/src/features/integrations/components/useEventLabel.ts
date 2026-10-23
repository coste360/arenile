import { useTranslation } from 'react-i18next';
import { ALL_EVENTS } from '../lib/events';

/** Translated label of an event filter: `*`, `<group>.*` or a single event type. */
export function useEventLabel() {
  const { t } = useTranslation('integrations');
  return (filter: string) => {
    if (filter === ALL_EVENTS) return t('events.all');
    if (filter.endsWith('.*')) {
      return t('events.group', { group: t(`groups.${filter.slice(0, -2)}`) });
    }
    return t(`events.types.${filter}`, { defaultValue: filter });
  };
}
