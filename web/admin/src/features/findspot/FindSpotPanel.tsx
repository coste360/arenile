import { Badge } from '@arenile/shared/ui/badge';
import { EmptyState } from '@arenile/shared/ui/patterns';
import { SearchIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Search form and ranked proposals of "Trova posto" (spec 30.6). Rendered in
 * the find-spot page and, by the map and the new booking, in a sheet.
 * Placeholder until the find-spot builder lands.
 */
export function FindSpotPanel() {
  const { t } = useTranslation('findspot');
  return (
    <EmptyState
      icon={SearchIcon}
      title={t('placeholder.title')}
      description={t('placeholder.description')}
    >
      <Badge variant="secondary">M8</Badge>
    </EmptyState>
  );
}
