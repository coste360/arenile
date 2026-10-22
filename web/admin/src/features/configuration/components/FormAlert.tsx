import { Button } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import type { FormAlertState } from '../lib/formErrors';

export interface FormAlertProps {
  state: FormAlertState | null;
  /** Reloads the server version (shown for 412 version_conflict). */
  onReload?: () => void;
}

/** Non-field errors of a form; for a stale ETag it offers to reload the data. */
export function FormAlert({ state, onReload }: FormAlertProps) {
  const { t } = useTranslation('configuration');
  if (!state) return null;
  return (
    <div
      role="alert"
      className="space-y-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800"
    >
      {state.kind === 'conflict' && (
        <>
          <p>{t('common.errors.versionConflict', { ns: 'translation' })}</p>
          {onReload && (
            <Button size="sm" variant="outline" onClick={onReload}>
              {t('conflict.reload')}
            </Button>
          )}
        </>
      )}
      {state.kind === 'message' && <p>{t(state.key, { ns: 'translation' })}</p>}
      {state.kind === 'server' && (
        <ul className="list-inside list-disc">
          {state.messages.map((m) => (
            <li key={m}>{m}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** Inline success confirmation (polite live region). */
export function SavedNotice({ show }: { show: boolean }) {
  const { t } = useTranslation();
  return (
    <span role="status" aria-live="polite" className="text-sm text-emerald-700">
      {show ? t('common.actions.saved') : ''}
    </span>
  );
}
