import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Page } from '@arenile/shared/ui/patterns';
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

/** Configuration page: the shared <Page> skeleton plus the read-only notice. */
export function ConfigPage({ title, description, actions, readOnly, children }: ConfigPageProps) {
  const { t } = useTranslation('configuration');
  return (
    <Page
      title={title}
      description={description}
      actions={actions}
      notice={readOnly && <ReadOnlyNotice text={t('readOnly')} />}
    >
      {children}
    </Page>
  );
}

/** "Sola lettura" banner of the configuration and pricing pages. */
export function ReadOnlyNotice({ text }: { text: string }) {
  return (
    <Alert role="note">
      <EyeIcon aria-hidden="true" />
      <AlertDescription>{text}</AlertDescription>
    </Alert>
  );
}
