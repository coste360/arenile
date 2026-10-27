import { useProblemText } from '@arenile/shared/api';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useUndoMove, type MoveUndoInfo } from '../api/m8';

/** The undo window of a move (spec 30.7), used when the server does not say. */
export const UNDO_WINDOW_MS = 5 * 60_000;

/** Milliseconds left to undo `info` at `now` (0 when expired). */
export function undoTimeLeft(info: Pick<MoveUndoInfo, 'expires_at'>, now = Date.now()): number {
  const at = Date.parse(info.expires_at);
  if (Number.isNaN(at)) return UNDO_WINDOW_MS;
  return Math.max(0, Math.min(at - now, UNDO_WINDOW_MS));
}

/**
 * Success toast of a move, a swap or a group move with an "Annulla" action
 * that stays until the undo window closes (5 minutes, spec 30.7). Undoing
 * answers 409 when the old spots were taken meanwhile and 410 after the
 * window: the error is shown as a toast and nothing changes.
 */
export function useUndoToast(establishmentId: string) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const undo = useUndoMove(establishmentId);
  return useCallback(
    (message: string, info: MoveUndoInfo | undefined) => {
      if (!info) {
        toast.success(message);
        return;
      }
      const left = undoTimeLeft(info);
      if (left <= 0) {
        toast.success(message);
        return;
      }
      toast.success(message, {
        duration: left,
        description: t('undo.hint'),
        action: {
          label: t('undo.action'),
          onClick: () =>
            undo.mutate(
              { moveId: info.move_id },
              {
                onSuccess: () => toast.success(t('undo.done')),
                onError: (e) => toast.error(problemText(e)),
              },
            ),
        },
      });
    },
    [t, undo, problemText],
  );
}
