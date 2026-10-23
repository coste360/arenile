import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import {
  ConfirmDialog,
  EmptyState,
  StatusBadge,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CloudOffIcon, RotateCcwIcon, SendIcon, Trash2Icon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useOfflineRuntime, useOfflineStatus } from './context';
import type { OfflineOperation } from './types';

const TONE: Record<OfflineOperation['status'], StatusTone> = {
  pending: 'neutral',
  sending: 'info',
  conflict: 'danger',
  failed: 'danger',
};

export interface OfflineQueueSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Operations recorded offline, in send order, with the server's refusal
 * for conflicts (spec 21.2: "eventuali conflitti vengono mostrati
 * all'utente"). Conflicts can be retried (new idempotency key) or discarded.
 */
export function OfflineQueueSheet({ open, onOpenChange }: OfflineQueueSheetProps) {
  const { t, i18n } = useTranslation('offline');
  const runtime = useOfflineRuntime();
  const { operations, online, pending } = useOfflineStatus();
  const [sending, setSending] = useState(false);

  const kindLabel = (op: OfflineOperation) =>
    op.kind === 'cash_payment'
      ? t('queue.kind.cash_payment', { amount: formatCents(op.amount) })
      : t(`queue.kind.${op.kind}`);
  const time = (iso: string) =>
    new Date(iso).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' });

  const sendNow = async () => {
    if (!runtime) return;
    setSending(true);
    try {
      await runtime.flush();
    } finally {
      setSending(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t('queue.title')}</SheetTitle>
          <SheetDescription>{t('queue.description')}</SheetDescription>
        </SheetHeader>
        <div className="px-4">
          {operations.length === 0 ? (
            <EmptyState icon={CloudOffIcon} title={t('queue.empty')} />
          ) : (
            <ul className="divide-y" aria-label={t('queue.title')}>
              {operations.map((op) => (
                <li key={op.id} className="space-y-2 py-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{kindLabel(op)}</p>
                      <p className="truncate text-sm text-muted-foreground">{op.label}</p>
                    </div>
                    <StatusBadge tone={TONE[op.status]}>
                      {t(`queue.status.${op.status}`)}
                    </StatusBadge>
                  </div>
                  {op.kind === 'note' && (
                    <p className="text-sm break-words whitespace-pre-line">{op.text}</p>
                  )}
                  <p className="text-xs text-muted-foreground">
                    {t('queue.queuedAt', { time: time(op.createdAt) })}
                  </p>
                  {(op.status === 'conflict' || op.status === 'failed') && (
                    <>
                      {op.error && <p className="text-sm text-destructive">{op.error}</p>}
                      <div className="flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            void runtime?.queue.retry(op.id).then(() => runtime.flush())
                          }
                        >
                          <RotateCcwIcon aria-hidden="true" />
                          {t('queue.retry')}
                        </Button>
                        <ConfirmDialog
                          title={t('queue.discardTitle')}
                          description={t('queue.discardDescription', {
                            kind: kindLabel(op),
                            label: op.label,
                          })}
                          confirmLabel={t('queue.discard')}
                          destructive
                          onConfirm={() => runtime?.queue.discard(op.id)}
                          trigger={
                            <Button size="sm" variant="ghost">
                              <Trash2Icon aria-hidden="true" />
                              {t('queue.discard')}
                            </Button>
                          }
                        />
                      </div>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
        {pending > 0 && (
          <SheetFooter>
            <Button onClick={() => void sendNow()} disabled={!online || sending}>
              {sending ? <Spinner /> : <SendIcon aria-hidden="true" />}
              {t('queue.sendNow')}
            </Button>
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  );
}
