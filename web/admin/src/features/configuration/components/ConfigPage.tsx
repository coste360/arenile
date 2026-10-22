import { ApiProblem } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui';
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

/** Layout of a configuration page. */
export function ConfigPage({ title, description, actions, readOnly, children }: ConfigPageProps) {
  const { t } = useTranslation('configuration');
  return (
    <section aria-labelledby="page-title" className="mx-auto max-w-5xl space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 id="page-title" className="text-2xl font-semibold">
            {title}
          </h1>
          {description && <p className="text-sm text-slate-600">{description}</p>}
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </header>
      {readOnly && (
        <p
          role="note"
          className="rounded-md border border-slate-200 bg-slate-100 px-3 py-2 text-sm text-slate-700"
        >
          {t('readOnly')}
        </p>
      )}
      {children}
    </section>
  );
}

/** Loading / error placeholder for a query. */
export function QueryState({
  isLoading,
  error,
  onRetry,
}: {
  isLoading: boolean;
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-red-700">
        <span>{t(problemMessageKey(error instanceof ApiProblem ? error : undefined))}</span>
        {onRetry && (
          <Button size="sm" variant="outline" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        )}
      </div>
    );
  }
  if (isLoading) return <p className="text-sm text-slate-600">{t('common.loading')}</p>;
  return null;
}

/** Empty state of a list. */
export function EmptyState({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-md border border-dashed border-slate-300 px-4 py-6 text-center text-sm text-slate-600">
      {children}
    </p>
  );
}

/** Small pill. */
export function Badge({
  children,
  tone = 'slate',
}: {
  children: ReactNode;
  tone?: 'slate' | 'green' | 'amber' | 'blue';
}) {
  const tones = {
    slate: 'bg-slate-100 text-slate-700',
    green: 'bg-emerald-100 text-emerald-800',
    amber: 'bg-amber-100 text-amber-900',
    blue: 'bg-sky-100 text-sky-900',
  };
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${tones[tone]}`}
    >
      {children}
    </span>
  );
}
