import { BackLink, EmptyState, Page } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { ArrowLeftIcon, ReceiptTextIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';

/** Partner detail with statement and payments (spec 30.2.4): placeholder. */
export function PartnerDetailPage() {
  const { t } = useTranslation('partners');
  const establishmentId = useRequiredEstablishmentId();
  return (
    <Page
      title={t('detail.title')}
      eyebrow={
        <BackLink>
          <Link to="/e/$establishmentId/convenzioni" params={{ establishmentId }}>
            <ArrowLeftIcon aria-hidden="true" />
            {t('detail.back')}
          </Link>
        </BackLink>
      }
    >
      <EmptyState
        icon={ReceiptTextIcon}
        title={t('detail.statement')}
        description={t('placeholder.description')}
      />
    </Page>
  );
}
