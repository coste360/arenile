import { Button } from '@arenile/shared/ui';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../api/useConfigContext';
import { ConfigPage, QueryState } from '../components/ConfigPage';
import {
  GeneralSection,
  PricingSettingsSection,
  PublicPageSection,
  ThemeSection,
} from './EstablishmentSections';

/** Configurazione → Stabilimento (spec 15.1, 12.5, 8.1/8.5). */
export function EstablishmentSettingsPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  // Version the editing session started from; see useEstablishmentSection.
  const [known, setKnown] = useState<{ id: string; version: number } | null>(null);
  const est = ctx.establishment;
  // Pin the version when an establishment is first shown (render-time derived state):
  // background refetches must not silently move the If-Match base.
  if (est && known?.id !== est.id) setKnown({ id: est.id, version: est.version });

  if (!est) {
    return (
      <ConfigPage title={t('establishment.title')}>
        <QueryState isLoading={ctx.isLoading} error={ctx.error} onRetry={ctx.refetch} />
      </ConfigPage>
    );
  }

  const props = {
    establishment: est,
    version: known?.id === est.id ? known.version : est.version,
    onVersion: (version: number) => setKnown({ id: est.id, version }),
    readOnly: !ctx.canConfigure,
  };

  return (
    <ConfigPage
      title={t('establishment.title')}
      description={t('establishment.description')}
      readOnly={!ctx.canConfigure}
      actions={
        ctx.canConfigure ? (
          <Button asChild variant="outline" size="sm">
            <Link to="/e/$establishmentId/setup" params={{ establishmentId: est.id }}>
              {t('establishment.openWizard')}
            </Link>
          </Button>
        ) : undefined
      }
    >
      {/* Keyed by id: switching establishment remounts the forms with fresh defaults. */}
      <div key={est.id} className="space-y-6">
        <GeneralSection {...props} />
        <PublicPageSection {...props} />
        <ThemeSection {...props} />
        <PricingSettingsSection {...props} />
      </div>
    </ConfigPage>
  );
}
