import { Badge } from '@arenile/shared/ui/badge';
import { EmptyState, PageHeader } from '@arenile/shared/ui/patterns';
import { ConstructionIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export interface PlaceholderPageProps {
  /** i18n key of the page title (nav.*). */
  titleKey: string;
  /** i18n key of the section description. */
  descriptionKey: string;
  /** Roadmap milestone that delivers the page (spec 28). */
  milestone: string;
  /** Render the title as <h1> with a page header (default true). */
  asPage?: boolean;
}

/** Honest placeholder for sections not delivered in the current milestone. */
export function PlaceholderPage({
  titleKey,
  descriptionKey,
  milestone,
  asPage = true,
}: PlaceholderPageProps) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby={asPage ? 'page-title' : undefined} className="space-y-6">
      {asPage && <PageHeader title={t(titleKey)} />}
      <EmptyState
        icon={ConstructionIcon}
        title={t('admin.placeholder.title')}
        description={t(descriptionKey)}
      >
        <Badge variant="outline" className="border-highlight bg-highlight/20 text-foreground">
          {t('common.comingSoon', { milestone })}
        </Badge>
      </EmptyState>
    </section>
  );
}
