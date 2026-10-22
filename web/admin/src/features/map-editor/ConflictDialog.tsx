import { Button, Dialog, DialogContent, DialogDescription, DialogTitle } from '@arenile/shared/ui';
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

function Labels({ labels }: { labels: string[] }) {
  const { t } = useTranslation('mapEditor');
  const shown = labels.slice(0, LIST_LIMIT).join(', ');
  const more = labels.length - LIST_LIMIT;
  return (
    <span className="text-slate-700">
      {shown}
      {more > 0 ? ` ${t('conflict.more', { count: more })}` : ''}
    </span>
  );
}

function DiffList({ diff, title }: { diff: DocumentDiff | null; title: string }) {
  const { t } = useTranslation('mapEditor');
  return (
    <section aria-label={title} className="space-y-2 rounded-md border border-slate-200 p-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      {!diff ? (
        <p className="text-sm text-slate-600">{t('conflict.unknown')}</p>
      ) : isEmptyDiff(diff) ? (
        <p className="text-sm text-slate-600">{t('conflict.none')}</p>
      ) : (
        <ul className="space-y-1 text-sm">
          {(['added', 'removed', 'moved', 'changed'] as const).map((k) =>
            diff[k].length > 0 ? (
              <li key={k}>
                <span className="font-medium">{t(`conflict.${k}`, { count: diff[k].length })}</span>{' '}
                <Labels labels={diff[k]} />
              </li>
            ) : null,
          )}
          {diff.decorations > 0 && (
            <li>{t('conflict.decorations', { count: diff.decorations })}</li>
          )}
          {diff.settings && <li>{t('conflict.settings')}</li>}
        </ul>
      )}
    </section>
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
    <Dialog open={!!conflict} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        closeLabel={t('common.close', { ns: 'translation' })}
        className="max-h-[90vh] max-w-2xl overflow-y-auto"
      >
        <DialogTitle>{t('conflict.title')}</DialogTitle>
        <DialogDescription>{t('editor.conflict')}</DialogDescription>
        {conflict && (
          <div className="grid gap-3 sm:grid-cols-2">
            <DiffList diff={conflict.yours} title={t('conflict.yours')} />
            <DiffList
              diff={conflict.theirs}
              title={
                conflict.currentVersion != null
                  ? t('conflict.theirsVersion', { version: conflict.currentVersion })
                  : t('conflict.theirs')
              }
            />
          </div>
        )}
        <div className="flex flex-wrap justify-end gap-2 pt-2">
          <Button variant="outline" onClick={onClose} disabled={busy}>
            {t('conflict.keepEditing')}
          </Button>
          <Button variant="outline" onClick={onReload} disabled={busy}>
            {t('editor.reload')}
          </Button>
          <Button variant="destructive" onClick={onOverwrite} disabled={busy}>
            {t('editor.overwrite')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
