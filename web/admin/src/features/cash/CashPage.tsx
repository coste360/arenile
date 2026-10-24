import { ApiProblem } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import {
  EmptyState,
  OptionSelectMenu,
  Page,
  PageToolbar,
  QueryState,
  SectionCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  ArrowLeftRightIcon,
  EyeIcon,
  FileDownIcon,
  FileSpreadsheetIcon,
  HandCoinsIcon,
  LockIcon,
  MoreHorizontalIcon,
  PrinterIcon,
  ReceiptTextIcon,
  SettingsIcon,
  WalletIcon,
} from 'lucide-react';
import { useState } from 'react';
import { flushSync } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useCurrentSession, useRegisters, useSession, type CashSessionDetail } from './api';
import { CloseSessionDialog } from './components/CloseSessionDialog';
import { MovementDialog } from './components/MovementDialog';
import { OpenSessionCard } from './components/OpenSessionCard';
import { QuickPaymentDialog } from './components/QuickPaymentDialog';
import { SessionHistory } from './components/SessionHistory';
import { SessionView } from './components/SessionView';
import { ZPdfButton } from './components/ZPdfButton';
import { ZReport } from './components/ZReport';
import { printZReport, zReportUrl } from './lib/zreport';
import { useCashContext, type CashContext } from './useCashContext';
import {
  pickRegister,
  readPreferredRegister,
  sessionRegisterLabel,
  writePreferredRegister,
} from './lib/register';

type DialogName = 'movement' | 'pay' | 'close' | null;

/**
 * Cassa (spec 14.1): open the session with a float, follow takings and
 * movements, take a payment for a booking, close with the count and print
 * the Z summary (quick print or the server PDF); history of the sessions
 * below. With several registers (ADR 0019) a picker chooses the register of
 * this device (remembered locally); each register has its own session.
 * Cashier: full access; viewer: read-only (history); other roles: no access.
 */
export function CashPage() {
  const { t } = useTranslation('cash');
  const ctx = useCashContext();
  const registers = useRegisters(ctx.establishmentId, false, !ctx.loading && ctx.canRead);
  const [preferred, setPreferred] = useState(() => readPreferredRegister(ctx.establishmentId));
  const regs = registers.data ?? [];
  const registerId = pickRegister(regs, preferred);
  const register = regs.find((r) => r.id === registerId);
  // Wait for the registers (or their failure: then the default register by code).
  const registersSettled = !registers.isPending || !!registers.error;
  const current = useCurrentSession(
    ctx.establishmentId,
    registerId,
    !ctx.loading && ctx.canRead && registersSettled,
  );
  const chooseRegister = (id: string) => {
    setPreferred(id);
    writePreferredRegister(ctx.establishmentId, id);
  };
  const [dialog, setDialog] = useState<DialogName>(null);
  const [historyId, setHistoryId] = useState<string | undefined>();
  const [closedId, setClosedId] = useState<string | undefined>();
  const [printDetail, setPrintDetail] = useState<CashSessionDetail | null>(null);

  // Render the Z summary into the print-only area synchronously, then print.
  const print = (d: CashSessionDetail) => {
    flushSync(() => setPrintDetail(d));
    window.print();
  };

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

  // Primary actions stay as buttons (Incassa, Chiudi cassa); the secondary
  // ones (corrispettivi, printouts, Z PDF, movement) live in one overflow
  // menu so that the header never squeezes the description on wide screens
  // nor wraps into a wall of buttons on phones.
  const printZ = (paper: 'a4' | 'receipt80') => {
    if (!detail) return;
    void printZReport(zReportUrl(ctx.establishmentId, detail.session.id, paper)).catch(() => {
      window.open(zReportUrl(ctx.establishmentId, detail.session.id, paper), '_blank', 'noopener');
    });
  };
  const hasMore = ctx.canReport || !!detail || operate;
  const actions = (
    <>
      {hasMore && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline">
              <MoreHorizontalIcon aria-hidden="true" />
              {t('actions.more')}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {ctx.canReport && (
              <DropdownMenuItem asChild>
                <Link
                  to="/e/$establishmentId/cassa/corrispettivi"
                  params={{ establishmentId: ctx.establishmentId }}
                >
                  <FileSpreadsheetIcon aria-hidden="true" />
                  {t('corrispettivi.title')}
                </Link>
              </DropdownMenuItem>
            )}
            {operate && (
              <DropdownMenuItem onSelect={() => setDialog('movement')}>
                <ArrowLeftRightIcon aria-hidden="true" />
                {t('actions.movement')}
              </DropdownMenuItem>
            )}
            {detail && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => print(detail)}>
                  <PrinterIcon aria-hidden="true" />
                  {t('actions.print')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => printZ('a4')}>
                  <FileDownIcon aria-hidden="true" />
                  {t('zpdf.a4')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => printZ('receipt80')}>
                  <ReceiptTextIcon aria-hidden="true" />
                  {t('zpdf.receipt')}
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {operate && (
        <>
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
                register: sessionRegisterLabel(t, detail.session),
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
        {regs.length > 1 && (
          <PageToolbar>
            <OptionSelectMenu
              label={t('registerPicker.label')}
              value={registerId}
              onValueChange={chooseRegister}
              className="sm:w-72"
              options={regs.map((r) => ({
                value: r.id,
                label: r.open_session_id
                  ? t('registerPicker.open', { name: r.name })
                  : t('registerPicker.closed', { name: r.name }),
              }))}
            />
            {ctx.canConfigure && (
              <Button variant="ghost" asChild>
                <Link
                  to="/e/$establishmentId/config/cash-registers"
                  params={{ establishmentId: ctx.establishmentId }}
                >
                  <SettingsIcon aria-hidden="true" />
                  {t('registerPicker.manage')}
                </Link>
              </Button>
            )}
          </PageToolbar>
        )}
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
              <OpenSessionCard
                establishmentId={ctx.establishmentId}
                registerId={register?.id}
                registerName={register && regs.length > 1 ? register.name : undefined}
              />
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
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => onPrint(detail.data)}>
              <PrinterIcon aria-hidden="true" />
              {t('actions.printZ')}
            </Button>
            <ZPdfButton establishmentId={ctx.establishmentId} sessionId={sessionId} />
          </div>
        )
      }
    >
      <QueryState isLoading={detail.isPending} error={detail.error} />
      {detail.data && <ZReport detail={detail.data} ctx={ctx} />}
    </SectionCard>
  );
}
