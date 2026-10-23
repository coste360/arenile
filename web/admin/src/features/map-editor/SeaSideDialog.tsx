import { problemMessageKey } from '@arenile/shared/api';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@arenile/shared/ui/alert-dialog';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CircleAlertIcon, InfoIcon, TriangleAlertIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useReorientMap } from './api';
import type { MapReorientResult, SeaSide } from './model/types';

/** Why the layout cannot be flipped from the dialog (only the sea can move). */
export type FlipBlock = 'dirty' | 'resized' | null;

export interface SeaSideDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  establishmentId: string;
  mapId: string;
  /** Saved version of the map (If-Match of the reorientation). */
  version: number;
  from: SeaSide;
  to: SeaSide;
  block: FlipBlock;
  /** "Solo il mare": the sea side changes in the draft, the layout stays. */
  onOnlySea: () => void;
  /** "Ribalta la disposizione": saved on the server as a new version. */
  onFlipped: (result: MapReorientResult) => void;
}

/**
 * Asked when the sea side of a map with elements changes (spec 11): flip or
 * rotate the whole layout so that every row keeps its distance from the sea
 * (one versioned save on the server, geometry only), or move only the sea.
 * A dry run first counts the future allocations of the map, which the flip
 * then acknowledges explicitly. Mount it only while a change is pending (the
 * page renders it with `open`): the dry run runs on mount.
 */
export function SeaSideDialog({
  open,
  onOpenChange,
  establishmentId,
  mapId,
  version,
  from,
  to,
  block,
  onOnlySea,
  onFlipped,
}: SeaSideDialogProps) {
  const { t } = useTranslation('mapEditor');
  const preview = useReorientMap(establishmentId, mapId);
  const apply = useReorientMap(establishmentId, mapId);
  const [error, setError] = useState<string | null>(null);
  const { mutate: runPreview } = preview;

  // The page mounts the dialog for one sea side change: count the future
  // allocations once (dry run, nothing is written).
  useEffect(() => {
    if (!block) {
      runPreview({
        version,
        body: { sea_side: to, dry_run: true, acknowledge_future_allocations: false },
      });
    }
  }, [block, to, version, runPreview]);

  const future = preview.data?.future_allocations ?? 0;
  const flip = async () => {
    setError(null);
    try {
      const res = await apply.mutateAsync({
        version,
        body: { sea_side: to, dry_run: false, acknowledge_future_allocations: future > 0 },
      });
      onFlipped(res);
      onOpenChange(false);
    } catch (err) {
      setError(t(problemMessageKey(err), { ns: 'translation' }));
    }
  };

  const busy = apply.isPending;
  const sides = { from: t(`seaSides.${from}`), to: t(`seaSides.${to}`) };
  return (
    <AlertDialog open={open} onOpenChange={(v) => !busy && onOpenChange(v)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('seaChange.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('seaChange.description', sides)}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3 text-sm">
          <p>
            <span className="font-medium">{t('seaChange.flip')}: </span>
            <span className="text-muted-foreground">{t('seaChange.flipHint')}</span>
          </p>
          <p>
            <span className="font-medium">{t('seaChange.onlySea')}: </span>
            <span className="text-muted-foreground">{t('seaChange.onlySeaHint')}</span>
          </p>
          {block && (
            <Alert role="note">
              <InfoIcon aria-hidden="true" />
              <AlertDescription>{t(`seaChange.blocked.${block}`)}</AlertDescription>
            </Alert>
          )}
          {!block && preview.isPending && (
            <p className="flex items-center gap-2 text-muted-foreground" aria-live="polite">
              <Spinner />
              {t('seaChange.checking')}
            </p>
          )}
          {future > 0 && (
            <Alert>
              <TriangleAlertIcon aria-hidden="true" />
              <AlertTitle>{t('seaChange.futureTitle')}</AlertTitle>
              <AlertDescription>
                {t('seaChange.future', {
                  count: future,
                  spots: preview.data?.spots_with_future_allocations ?? 0,
                })}
              </AlertDescription>
            </Alert>
          )}
          {(error || preview.error) && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>
                {error ?? t(problemMessageKey(preview.error), { ns: 'translation' })}
              </AlertDescription>
            </Alert>
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </AlertDialogCancel>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => {
              onOnlySea();
              onOpenChange(false);
            }}
          >
            {t('seaChange.onlySea')}
          </Button>
          <Button disabled={!!block || busy || !preview.isSuccess} onClick={() => void flip()}>
            {busy && <Spinner />}
            {t('seaChange.flip')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
