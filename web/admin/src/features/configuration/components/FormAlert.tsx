import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { CircleAlertIcon, RotateCcwIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { FormAlertState } from '../lib/formErrors';

export interface FormAlertProps {
  state: FormAlertState | null;
  /** Reloads the server version (shown for 412 version_conflict). */
  onReload?: () => void;
}

/** Non-field errors of a form (shadcn Alert); for a stale ETag it offers to reload the data. */
export function FormAlert({ state, onReload }: FormAlertProps) {
  const { t } = useTranslation('configuration');
  if (!state) return null;
  return (
    <Alert variant="destructive">
      <CircleAlertIcon aria-hidden="true" />
      {state.kind === 'conflict' && (
        <>
          <AlertTitle>{t('common.errors.versionConflict', { ns: 'translation' })}</AlertTitle>
          {onReload && (
            <AlertDescription>
              <Button size="sm" variant="outline" className="mt-2" onClick={onReload}>
                <RotateCcwIcon aria-hidden="true" />
                {t('conflict.reload')}
              </Button>
            </AlertDescription>
          )}
        </>
      )}
      {state.kind === 'message' && (
        <AlertDescription>{t(state.key, { ns: 'translation' })}</AlertDescription>
      )}
      {state.kind === 'server' && (
        <AlertDescription>
          <ul className="list-inside list-disc">
            {state.messages.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        </AlertDescription>
      )}
    </Alert>
  );
}
