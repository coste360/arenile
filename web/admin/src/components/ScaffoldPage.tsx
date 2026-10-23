import { EmptyState, PageHeader } from '@arenile/shared/ui/patterns';
import { ConstructionIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

export interface ScaffoldPageProps {
  /** Translated page title (the page's only <h1>). */
  title: string;
  /** Translated description of what the page will do. */
  description: string;
  /** Header actions, if any. */
  actions?: ReactNode;
  children?: ReactNode;
}

/**
 * Temporary body of the M3 feature pages while their builders implement
 * them: the final page header plus an honest "in preparation" state. Feature
 * builders replace the usage with the real page and never edit this file.
 */
export function ScaffoldPage({ title, description, actions, children }: ScaffoldPageProps) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="page-title" className="space-y-6">
      <PageHeader title={title} description={description} actions={actions} />
      {children ?? (
        <EmptyState
          icon={ConstructionIcon}
          title={t('admin.placeholder.title')}
          description={description}
        />
      )}
    </section>
  );
}
