import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';

export interface PlaceholderPageProps {
  /** i18n key of the page title (nav.*). */
  titleKey: string;
  /** i18n key of the section description. */
  descriptionKey: string;
  /** Roadmap milestone that delivers the page (spec 28). */
  milestone: string;
}

/** Honest placeholder for sections not delivered in the current milestone. */
export function PlaceholderPage({ titleKey, descriptionKey, milestone }: PlaceholderPageProps) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="page-title" className="space-y-4">
      <h1 id="page-title" className="text-2xl font-semibold">
        {t(titleKey)}
      </h1>
      <Card>
        <CardHeader>
          <CardTitle>{t('admin.placeholder.title')}</CardTitle>
          <CardDescription>{t(descriptionKey)}</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="inline-flex rounded-full bg-slate-100 px-3 py-1 text-sm font-medium text-slate-700">
            {t('common.comingSoon', { milestone })}
          </p>
        </CardContent>
      </Card>
    </section>
  );
}
