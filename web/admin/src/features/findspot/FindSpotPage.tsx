import { Page } from '@arenile/shared/ui/patterns';
import { useSearch } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { FindSpotPanel } from './FindSpotPanel';

/** "Trova posto" as a page (spec 30.6); `?date=&slot=` prefill the search. */
export function FindSpotPage() {
  const { t } = useTranslation('findspot');
  const search = useSearch({ strict: false }) as { date?: string; slot?: 'FULL' | 'AM' | 'PM' };
  return (
    <Page title={t('title')} description={t('description')}>
      <FindSpotPanel
        defaults={{ startsOn: search.date, endsOn: search.date, slots: search.slot }}
      />
    </Page>
  );
}
