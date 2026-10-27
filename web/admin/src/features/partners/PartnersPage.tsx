import { Badge } from '@arenile/shared/ui/badge';
import { EmptyState, Page } from '@arenile/shared/ui/patterns';
import { HandshakeIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** "Partner e convenzioni" (spec 30.2): placeholder until the M8 builders land. */
export function PartnersPage() {
  const { t } = useTranslation('partners');
  return (
    <Page title={t('title')} description={t('description')}>
      <EmptyState
        icon={HandshakeIcon}
        title={t('placeholder.title')}
        description={t('placeholder.description')}
      >
        <Badge variant="secondary">M8</Badge>
      </EmptyState>
    </Page>
  );
}
