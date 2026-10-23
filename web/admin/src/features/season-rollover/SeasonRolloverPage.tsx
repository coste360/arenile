import { BackLink, EmptyState, Page } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { ArrowLeftIcon, ConstructionIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';

/**
 * End-of-season wizard (spec 8.7, 10.4, 28): source season and target year,
 * preview (dry run), apply, then link to the renewal campaign. Reached from
 * the seasons page (`?seasonId=` = source season). Scaffold: the season
 * rollover frontend builder replaces the body.
 */
export function SeasonRolloverPage() {
  const { t } = useTranslation('seasonRollover');
  const { t: tc } = useTranslation();
  const establishmentId = useRequiredEstablishmentId();
  return (
    <Page
      title={t('title')}
      description={t('description')}
      eyebrow={
        <BackLink>
          <Link to="/e/$establishmentId/config/seasons" params={{ establishmentId }}>
            <ArrowLeftIcon aria-hidden="true" />
            {t('back')}
          </Link>
        </BackLink>
      }
    >
      <EmptyState
        icon={ConstructionIcon}
        title={tc('admin.placeholder.title')}
        description={t('description')}
      />
    </Page>
  );
}
