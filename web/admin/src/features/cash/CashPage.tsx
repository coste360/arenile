import { ApiProblem } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  EmptyState,
  Page,
  QueryState,
  SectionCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  ArrowLeftRightIcon,
  EyeIcon,
  FileSpreadsheetIcon,
  HandCoinsIcon,
  LockIcon,
  PrinterIcon,
  WalletIcon,
} from 'lucide-react';
import { useState } from 'react';
import { flushSync } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useCurrentSession, useSession, type CashSessionDetail } from './api';
import { CloseSessionDialog } from './components/CloseSessionDialog';
import { MovementDialog } from './components/MovementDialog';
import { OpenSessionCard } from './components/OpenSessionCard';
import { QuickPaymentDialog } from './components/QuickPaymentDialog';
import { SessionHistory } from './components/SessionHistory';
import { SessionView } from './components/SessionView';
import { ZReport } from './components/ZReport';
import { usePrintDocument } from '../printers';
import { useCashContext, type CashContext } from './useCashContext';
import { registerLabel } from './lib/register';

type DialogName = 'movement' | 'pay' | 'close' | null;

/**
 * Cassa (spec 14.1): open the session with a float, follow takings and
 * movements, take a payment for a booking, close with the count and print
 * the Z summary; history of the sessions below. Cashier: full access;
 * viewer: read-only (history); other roles: no access.
 */
export function CashPage() {
  const { t } = useTranslation('cash');
  const ctx = useCashContext();
  const current = useCurrentSession(ctx.establishmentId, undefined, !ctx.loading && ctx.canRead);
  const [dialog, setDialog] = useState<DialogName>(null);
  const [historyId, setHistoryId] = useState<string | undefined>();
  const [closedId, setClosedId] = useState<string | undefined>();
  const [printDetail, setPrintDetail] = useState<CashSessionDetail | null>(null);

  const printDocument = usePrintDocument();
  // Render the Z summary into the print-only area synchronously, then print.
  const browserPrint = (d: CashSessionDetail) => {
    flushSync(() => setPrintDetail(d));
    window.print();
  };
  // ESC/POS on the routed thermal printer (desktop edition), browser otherwise (ADR 0021).
  const print = (d: CashSessionDetail) =>
    void printDocument(
      { kind: 'cash_z', documentId: d.session.id, registerId: d.session.register_id },
      () => browserPrint(d),
    );

  if (ctx.loading) {
    return <QueryState isLoading error={null} rows={4} />;
  }
  if (!ctx.canRead) {
    return (
      <Page title={t('session.title')} description={t('session.description')}>
        <Alert>
          <LockIcon aria-hidden="true" />
          <AlertTitle>{t('perm.forbiddenTitle')}</AlertTitle>
          <AlertDescription>{t('perm.forbidden')}</AlertDescription>
        </Alert>
      </Page>
    );
  }

  // A read-only principal may not be allowed to read the live session (403): show history only.
  const forbidden = current.error instanceof ApiProblem && current.error.status === 403;
  const detail = current.data ?? null;
  const operate = ctx.canOperate && !!detail;

  const actions = (
    <>
      {ctx.canReport && (
        <Button variant="outline" asChild>
          <Link
            to="/e/$establishmentId/cassa/corrispettivi"
            params={{ establishmentId: ctx.establishmentId }}
          >
            <FileSpreadsheetIcon aria-hidden="true" />
            {t('corrispettivi.title')}
          </Link>
        </Button>
      )}
      {detail && (
        <Button variant="outline" onClick={() => print(detail)}>
          <PrinterIcon aria-hidden="true" />
          {t('actions.print')}
        </Button>
      )}
      {operate && (
        <>
          <Button variant="outline" onClick={() => setDialog('movement')}>
            <ArrowLeftRightIcon aria-hidden="true" />
            {t('actions.movement')}
          </Button>
          <Button variant="outline" onClick={() => setDialog('close')}>
            <LockIcon aria-hidden="true" />
            {t('actions.close')}
          </Button>
          <Button onClick={() => setDialog('pay')}>
            <HandCoinsIcon aria-hidden="true" />
            {t('actions.quickPay')}
          </Button>
        </>
      )}
    </>
  );

  return (
    <>
      <Page
        className="print:hidden"
        title={t('session.title')}
        description={t('session.description')}
        actions={actions}
        eyebrow={
          detail ? (
            <StatusBadge tone="success">
              {t('current.badge', {
                register: registerLabel(t, detail.session.register),
                time: ctx.dateTime(detail.session.opened_at),
              })}
            </StatusBadge>
          ) : undefined
        }
        notice={
          !ctx.canOperate && (
            <Alert>
              <EyeIcon aria-hidden="true" />
              <AlertDescription>{t('perm.readOnly')}</AlertDescription>
            </Alert>
          )
        }
      >
        {forbidden ? null : current.isPending || current.error ? (
          <QueryState
            isLoading={current.isPending}
            error={current.error}
            onRetry={() => void current.refetch()}
            rows={4}
          />
        ) : detail ? (
          <SessionView detail={detail} ctx={ctx} />
        ) : (
          <div className="grid items-start gap-6 lg:grid-cols-2 *:min-w-0">
            {closedId && <ClosedSummary ctx={ctx} sessionId={closedId} onPrint={print} />}
            {ctx.canOperate ? (
              <OpenSessionCard establishmentId={ctx.establishmentId} />
            ) : (
              <EmptyState icon={WalletIcon} title={t('open.noneReadOnly')} />
            )}
          </div>
        )}

        <SessionHistory ctx={ctx} selectedId={historyId} onSelect={setHistoryId} onPrint={print} />
      </Page>

      <div className="hidden print:block" data-testid="print-area">
        {printDetail && <ZReport detail={printDetail} ctx={ctx} />}
      </div>

      {operate && (
        <>
          <MovementDialog
            establishmentId={ctx.establishmentId}
            sessionId={detail.session.id}
            open={dialog === 'movement'}
            onOpenChange={(o) => setDialog(o ? 'movement' : null)}
          />
          <QuickPaymentDialog
            ctx={ctx}
            sessionId={detail.session.id}
            registerId={detail.session.register_id}
            open={dialog === 'pay'}
            onOpenChange={(o) => setDialog(o ? 'pay' : null)}
          />
          <CloseSessionDialog
            ctx={ctx}
            detail={detail}
            open={dialog === 'close'}
            onOpenChange={(o) => setDialog(o ? 'close' : null)}
            onClosed={(s) => setClosedId(s.id)}
          />
        </>
      )}
    </>
  );
}

/** Z summary of the session just closed, with print, next to the "open" card. */
function ClosedSummary({
  ctx,
  sessionId,
  onPrint,
}: {
  ctx: CashContext;
  sessionId: string;
  onPrint: (d: CashSessionDetail) => void;
}) {
  const { t } = useTranslation('cash');
  const detail = useSession(ctx.establishmentId, sessionId);
  return (
    <SectionCard
      title={t('close.closedTitle')}
      titleId="closed-summary-title"
      actions={
        detail.data && (
          <Button variant="outline" size="sm" onClick={() => onPrint(detail.data)}>
            <PrinterIcon aria-hidden="true" />
            {t('actions.printZ')}
          </Button>
        )
      }
    >
      <QueryState isLoading={detail.isPending} error={detail.error} />
      {detail.data && <ZReport detail={detail.data} ctx={ctx} />}
    </SectionCard>
  );
}
