import { Button } from '@arenile/shared/ui/button';
import { ArrowRightIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PublicPageSection, ThemeSection } from '../../configuration/pages/EstablishmentSections';
import type { StepProps } from './types';

/** Step 5: public page — activation, theme, embed code. */
export function StepPublicPage({ ctx, dispatch, version, onVersion }: StepProps) {
  const { t } = useTranslation('setupWizard');
  if (!ctx.establishment) return null;
  const props = { establishment: ctx.establishment, version, onVersion, readOnly: false };
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('publicPage.intro')}</p>
      <PublicPageSection {...props} />
      <ThemeSection {...props} />
      <Button onClick={() => dispatch({ type: 'complete' })}>
        {t('continue')}
        <ArrowRightIcon aria-hidden="true" />
      </Button>
    </div>
  );
}
