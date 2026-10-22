import { Button } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../lib/problem';

export function RouteError({ error, reset }: { error: unknown; reset: () => void }) {
  const { t } = useTranslation();
  return (
    <div role="alert" className="mx-auto max-w-md space-y-3 p-6">
      <h1 className="text-xl font-semibold">{t('common.errors.title')}</h1>
      <p className="text-slate-700">{t(problemMessageKey(error))}</p>
      <Button onClick={reset}>{t('common.retry')}</Button>
    </div>
  );
}
