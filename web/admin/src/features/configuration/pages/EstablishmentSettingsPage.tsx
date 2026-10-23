import { Button } from '@arenile/shared/ui/button';
import { QueryState } from '@arenile/shared/ui/patterns';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import { Link } from '@tanstack/react-router';
import {
  Building2Icon,
  GlobeIcon,
  LanguagesIcon,
  ReceiptTextIcon,
  SparklesIcon,
  type LucideIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../api/useConfigContext';
import { ConfigPage } from '../components/ConfigPage';
import {
  GeneralSection,
  PricingSettingsSection,
  PublicPageSection,
  ThemeSection,
} from './EstablishmentSections';

const TABS: { id: string; icon: LucideIcon }[] = [
  { id: 'general', icon: Building2Icon },
  { id: 'languages', icon: LanguagesIcon },
  { id: 'publicPage', icon: GlobeIcon },
  { id: 'pricing', icon: ReceiptTextIcon },
];

/** Configurazione → Stabilimento (spec 15.1, 12.5, 8.1/8.5), one tab per area. */
export function EstablishmentSettingsPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const [tab, setTab] = useState('general');
  // Version the editing session started from; see useEstablishmentSection.
  const [known, setKnown] = useState<{ id: string; version: number } | null>(null);
  const est = ctx.establishment;
  // Pin the version when an establishment is first shown (render-time derived state):
  // background refetches must not silently move the If-Match base.
  if (est && known?.id !== est.id) setKnown({ id: est.id, version: est.version });

  if (!est) {
    return (
      <ConfigPage title={t('establishment.title')}>
        <QueryState isLoading={ctx.isLoading} error={ctx.error} onRetry={ctx.refetch} rows={6} />
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
          <Button asChild variant="outline">
            <Link to="/e/$establishmentId/setup" params={{ establishmentId: est.id }}>
              <SparklesIcon aria-hidden="true" />
              {t('establishment.openWizard')}
            </Link>
          </Button>
        ) : undefined
      }
    >
      {/* Keyed by id: switching establishment remounts the forms with fresh defaults.
          Every panel stays mounted (hidden) so unsaved edits survive a tab change. */}
      <Tabs key={est.id} value={tab} onValueChange={setTab} className="gap-6">
        <TabsList aria-label={t('establishment.tabsLabel')}>
          {TABS.map(({ id, icon: Icon }) => (
            <TabsTrigger key={id} value={id}>
              <Icon aria-hidden="true" />
              {t(`establishment.tabs.${id}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="general" forceMount hidden={tab !== 'general'}>
          <GeneralSection {...props} part="details" />
        </TabsContent>
        <TabsContent value="languages" forceMount hidden={tab !== 'languages'}>
          <GeneralSection {...props} part="languages" />
        </TabsContent>
        <TabsContent
          value="publicPage"
          forceMount
          hidden={tab !== 'publicPage'}
          className="space-y-6"
        >
          <PublicPageSection {...props} />
          <ThemeSection {...props} />
        </TabsContent>
        <TabsContent value="pricing" forceMount hidden={tab !== 'pricing'}>
          <PricingSettingsSection {...props} />
        </TabsContent>
      </Tabs>
    </ConfigPage>
  );
}
