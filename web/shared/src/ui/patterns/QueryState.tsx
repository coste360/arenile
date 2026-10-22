import { RotateCwIcon, TriangleAlertIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Skeleton } from '@arenile/shared/ui/skeleton';

export interface QueryStateProps {
  isLoading: boolean;
  error: unknown;
  /** Shown as a "Riprova" button inside the error alert. */
  onRetry?: () => void;
  /** Skeleton rows while loading (default 3). */
  rows?: number;
  /** Custom loading skeleton instead of the rows. */
  skeleton?: ReactNode;
}

/** Loading skeleton or error alert (with retry) of a TanStack Query; renders nothing when settled. */
export function QueryState({ isLoading, error, onRetry, rows = 3, skeleton }: QueryStateProps) {
  const { t } = useTranslation();
  if (error) {
    return (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>{t('common.errors.title')}</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center gap-3">
          <span>{t(problemMessageKey(error))}</span>
          {onRetry && (
            <Button size="sm" variant="outline" onClick={onRetry}>
              <RotateCwIcon aria-hidden="true" />
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
        {skeleton ??
          Array.from({ length: rows }, (_, i) => <Skeleton key={i} className="h-10 w-full" />)}
      </div>
    );
  }
  return null;
}
