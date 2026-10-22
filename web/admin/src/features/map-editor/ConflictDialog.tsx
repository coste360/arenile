import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Skeleton,
  Spinner,
} from '@arenile/shared/ui';
import {
  CircleCheckIcon,
  MinusIcon,
  MoveIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  Settings2Icon,
  ShapesIcon,
  UserIcon,
  UsersIcon,
  type LucideIcon,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { isEmptyDiff, type DocumentDiff } from './model/editor';

export interface ConflictInfo {
  /** Version now on the server (from the 412 problem or the re-read document). */
  currentVersion?: number;
  /** From the last loaded/saved version to the draft. */
  yours: DocumentDiff;
  /** From the last loaded/saved version to the server's current version. */
  theirs: DocumentDiff | null;
}

const LIST_LIMIT = 12;

const KIND_ICONS: Record<'added' | 'removed' | 'moved' | 'changed', LucideIcon> = {
  added: PlusIcon,
  removed: MinusIcon,
  moved: MoveIcon,
  changed: PencilIcon,
};

function Labels({ labels }: { labels: string[] }) {
  const { t } = useTranslation('mapEditor');
  const more = labels.length - LIST_LIMIT;
  return (
    <div className="flex flex-wrap gap-1">
      {labels.slice(0, LIST_LIMIT).map((l) => (
        <Badge key={l} variant="secondary" className="font-mono">
          {l}
        </Badge>
      ))}
      {more > 0 && (
        <span className="text-xs text-muted-foreground">{t('conflict.more', { count: more })}</span>
      )}
    </div>
  );
}

function DiffList({
  diff,
  title,
  icon: Icon,
}: {
  diff: DocumentDiff | null;
  title: string;
  icon: LucideIcon;
}) {
  const { t } = useTranslation('mapEditor');
  return (
    <Card role="region" aria-label={title} className="gap-3 py-4 shadow-none">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Icon aria-hidden="true" className="size-4 text-muted-foreground" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="px-4 text-sm">
        {!diff ? (
          <div className="space-y-2">
            <p className="text-muted-foreground">{t('conflict.unknown')}</p>
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        ) : isEmptyDiff(diff) ? (
          <p className="flex items-center gap-2 text-muted-foreground">
            <CircleCheckIcon aria-hidden="true" className="size-4" />
            {t('conflict.none')}
          </p>
        ) : (
          <ul className="space-y-3">
            {(['added', 'removed', 'moved', 'changed'] as const).map((k) => {
              if (diff[k].length === 0) return null;
              const KindIcon = KIND_ICONS[k];
              return (
                <li key={k} className="space-y-1.5">
                  <p className="flex items-center gap-1.5 font-medium">
                    <KindIcon aria-hidden="true" className="size-3.5 text-muted-foreground" />
                    <span>{t(`conflict.${k}`, { count: diff[k].length })}</span>
                  </p>
                  <Labels labels={diff[k]} />
                </li>
              );
            })}
            {diff.decorations > 0 && (
              <li className="flex items-center gap-1.5 text-muted-foreground">
                <ShapesIcon aria-hidden="true" className="size-3.5" />
                {t('conflict.decorations', { count: diff.decorations })}
              </li>
            )}
            {diff.settings && (
              <li className="flex items-center gap-1.5 text-muted-foreground">
                <Settings2Icon aria-hidden="true" className="size-3.5" />
                {t('conflict.settings')}
              </li>
            )}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Version conflict on save (412): what the user changed and what was saved
 * meanwhile, then reload (discard the draft) or overwrite (save the draft on
 * top of the server version).
 */
export function ConflictDialog({
  conflict,
  busy,
  onReload,
  onOverwrite,
  onClose,
}: {
  conflict: ConflictInfo | null;
  busy?: boolean;
  onReload: () => void;
  onOverwrite: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  return (
    <AlertDialog open={!!conflict} onOpenChange={(o) => !o && !busy && onClose()}>
      <AlertDialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{t('conflict.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('editor.conflict')}</AlertDialogDescription>
        </AlertDialogHeader>
        {conflict && (
          <div className="grid gap-3 sm:grid-cols-2">
            <DiffList diff={conflict.yours} title={t('conflict.yours')} icon={UserIcon} />
            <DiffList
              diff={conflict.theirs}
              icon={UsersIcon}
              title={
                conflict.currentVersion != null
                  ? t('conflict.theirsVersion', { version: conflict.currentVersion })
                  : t('conflict.theirs')
              }
            />
          </div>
        )}
        <AlertDialogFooter className="gap-2 sm:justify-between">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('conflict.keepEditing')}
          </Button>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="outline" onClick={onReload} disabled={busy}>
              <RefreshCwIcon aria-hidden="true" />
              {t('editor.reload')}
            </Button>
            <Button variant="destructive" onClick={onOverwrite} disabled={busy}>
              {busy && <Spinner />}
              {t('editor.overwrite')}
            </Button>
          </div>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
