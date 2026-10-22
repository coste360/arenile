import { useTranslation } from 'react-i18next';
import { GeneralSection } from '../../configuration/pages/EstablishmentSections';
import type { StepProps } from './types';

/** Step 1: establishment data and languages. */
export function StepDetails({ ctx, dispatch, version, onVersion }: StepProps) {
  const { t } = useTranslation('setupWizard');
  if (!ctx.establishment) return null;
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('details.intro')}</p>
      <GeneralSection
        establishment={ctx.establishment}
        version={version}
        onVersion={onVersion}
        readOnly={false}
        submitLabel={t('saveAndContinue')}
        onSaved={() => dispatch({ type: 'complete' })}
      />
    </div>
  );
}
