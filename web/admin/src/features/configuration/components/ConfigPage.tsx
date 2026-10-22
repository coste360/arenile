import { ApiProblem } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { PageHeader } from '@arenile/shared/ui/patterns';
import { EyeIcon, TriangleAlertIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../../../lib/problem';

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

/** Loading skeleton / error alert for a query. */
export function QueryState({
  isLoading,
  error,
  onRetry,
  rows = 3,
}: {
  isLoading: boolean;
  error: unknown;
  onRetry?: () => void;
  rows?: number;
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertDescription className="flex flex-wrap items-center gap-3">
          <span>{t(problemMessageKey(error instanceof ApiProblem ? error : undefined))}</span>
          {onRetry && (
            <Button size="sm" variant="outline" onClick={onRetry}>
              {t('common.retry')}
            </Button>
          )}
        </AlertDescription>
      </Alert>
    );
  }
  if (isLoading) {
    return (
      <div role="status" aria-live="polite" className="space-y-3">
        <span className="sr-only">{t('common.loading')}</span>
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }
  return null;
}
