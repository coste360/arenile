import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { CircleAlertIcon, RotateCcwIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ProblemText } from './problem';

/** Failure of a booking operation: friendly title, details and, when stale, a reload button. */
export function ProblemAlert({
  problem,
  onReload,
}: {
  problem: ProblemText | null;
  onReload?: () => void;
}) {
  const { t } = useTranslation('bookings');
  if (!problem) return null;
  return (
    <Alert variant="destructive" role="alert">
      <CircleAlertIcon aria-hidden="true" />
      <AlertTitle>{problem.title}</AlertTitle>
      {(problem.details.length > 0 || (problem.reload && onReload)) && (
        <AlertDescription>
          {problem.details.length > 0 && (
            <ul className="list-inside list-disc">
              {problem.details.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          )}
          {problem.reload && onReload && (
            <Button size="sm" variant="outline" className="mt-2" onClick={onReload}>
              <RotateCcwIcon aria-hidden="true" />
              {t('detail.reload')}
            </Button>
          )}
        </AlertDescription>
      )}
    </Alert>
  );
}
