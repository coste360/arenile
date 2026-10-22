import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { PageHeader } from '@arenile/shared/ui/patterns';
import { EyeIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

export interface ConfigPageProps {
  title: string;
  description?: string;
  actions?: ReactNode;
  /** Shows the read-only banner (roles without configuration permission). */
  readOnly?: boolean;
  children: ReactNode;
}

/** Layout of a configuration page: shared PageHeader, read-only notice, content. */
export function ConfigPage({ title, description, actions, readOnly, children }: ConfigPageProps) {
  const { t } = useTranslation('configuration');
  return (
    <section aria-labelledby="page-title" className="mx-auto max-w-6xl space-y-6">
      <PageHeader title={title} description={description} actions={actions} />
      {readOnly && (
        <Alert role="note">
          <EyeIcon aria-hidden="true" />
          <AlertDescription>{t('readOnly')}</AlertDescription>
        </Alert>
      )}
      {children}
    </section>
  );
}
