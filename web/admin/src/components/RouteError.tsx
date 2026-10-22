import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { AlertCircleIcon, RotateCwIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../lib/problem';

export function RouteError({ error, reset }: { error: unknown; reset: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="mx-auto max-w-md space-y-4 p-6">
      <Alert variant="destructive">
        <AlertCircleIcon aria-hidden="true" />
        <AlertTitle>
          <h1>{t('common.errors.title')}</h1>
        </AlertTitle>
        <AlertDescription>{t(problemMessageKey(error))}</AlertDescription>
      </Alert>
      <Button onClick={reset}>
        <RotateCwIcon aria-hidden="true" />
        {t('common.retry')}
      </Button>
    </div>
  );
}
