import { Badge } from '@arenile/shared/ui/badge';
import { BackLink, EmptyState, Page } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { ArrowLeftIcon, FileSpreadsheetIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';

/** Report per partner and agreements register (spec 30.2.5): placeholder. */
export function AgreementsReportsPage() {
  const { t } = useTranslation('agreementsReports');
  const { t: tp } = useTranslation('partners');
  const establishmentId = useRequiredEstablishmentId();
  return (
    <Page
      title={t('title')}
      description={t('description')}
      eyebrow={
        <BackLink>
          <Link to="/e/$establishmentId/convenzioni" params={{ establishmentId }}>
            <ArrowLeftIcon aria-hidden="true" />
            {tp('title')}
          </Link>
        </BackLink>
      }
    >
      <EmptyState
        icon={FileSpreadsheetIcon}
        title={t('placeholder.title')}
        description={t('placeholder.description')}
      >
        <Badge variant="secondary">M8</Badge>
      </EmptyState>
    </Page>
  );
}
