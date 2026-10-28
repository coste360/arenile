import { useProblemText } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Undo2Icon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useLastMove, useUndoMove } from '../api/m8';
import { undoTimeLeft } from './useUndoToast';

/**
 * "Annulla ultimo spostamento" (spec 30.7): the current user's last move,
 * swap or group move of the last 5 minutes, still undoable. Hidden when
 * there is none or the window has closed (it hides itself on time).
 */
export function UndoLastMoveButton({
  establishmentId,
  enabled,
}: {
  establishmentId: string;
  enabled: boolean;
}) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const last = useLastMove(establishmentId, enabled);
  const undo = useUndoMove(establishmentId);
  const move = last.data?.move;
  const [, tick] = useState(0);
  const left = move ? undoTimeLeft(move) : 0;
  useEffect(() => {
    if (!move || left <= 0) return;
    const id = window.setTimeout(() => tick((n) => n + 1), left + 50);
    return () => window.clearTimeout(id);
  }, [move, left]);
  if (!move || left <= 0) return null;
  return (
    <Button
      variant="outline"
      disabled={undo.isPending}
      onClick={() =>
        undo.mutate(
          { moveId: move.move_id },
          {
            onSuccess: () => toast.success(t('undo.done')),
            onError: (e) => toast.error(problemText(e)),
          },
        )
      }
    >
      {undo.isPending ? <Spinner /> : <Undo2Icon aria-hidden="true" />}
      <span className="sr-only sm:not-sr-only">{t('undo.last')}</span>
    </Button>
  );
}
