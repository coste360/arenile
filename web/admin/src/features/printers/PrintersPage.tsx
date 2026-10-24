import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  DeleteButton,
  EditButton,
  EmptyState,
  FormSheet,
  Page,
  QueryState,
  SectionCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { MonitorIcon, PlusIcon, PrinterIcon, RadarIcon, ReceiptTextIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import { useOperationsPermissions } from '../operations/lib/permissions';
import {
  useCashRegisters,
  useCheckPrinter,
  useDeletePrinter,
  usePrintRoutes,
  usePrinters,
  useTestPrinter,
  type Printer,
} from './api';
import { JobsCard } from './JobsCard';
import { PRINTER_STATUS_TONE, printerAddress } from './lib';
import { PrinterForm } from './PrinterForm';
import { RoutingCard } from './RoutingCard';
import { usePrintingProblemText } from './usePrintDocument';

/**
 * Configurazione → Stampanti (ADR 0021): ESC/POS network printers (Wi-Fi or
 * Ethernet, raw TCP 9100) with reachability check and test print, document
 * routing per cash register and the latest print jobs. ESC/POS printing
 * runs only on the desktop edition (the server is in the lido and reaches
 * the printer on the LAN); on the server edition the page explains that
 * printing falls back to the browser.
 */
export function PrintersPage() {
  const { t } = useTranslation('printers');
  const establishmentId = useRequiredEstablishmentId();
  const perms = useOperationsPermissions();
  const printers = usePrinters(establishmentId);
  const available = printers.data?.escpos_available ?? false;
  const canConfigure = perms.canConfigure && available;
  const routes = usePrintRoutes(establishmentId, available);
  const registers = useCashRegisters(establishmentId, available);
  const [editing, setEditing] = useState<Printer | 'new' | null>(null);
  const list = printers.data?.items ?? [];

  return (
    <Page
      title={t('page.title')}
      description={t('page.description')}
      actions={
        canConfigure && (
          <Button onClick={() => setEditing('new')}>
            <PlusIcon aria-hidden="true" />
            {t('add')}
          </Button>
        )
      }
      notice={
        printers.data &&
        !available && (
          <Alert role="note">
            <MonitorIcon aria-hidden="true" />
            <AlertTitle>{t('unavailable.title')}</AlertTitle>
            <AlertDescription>{t('unavailable.description')}</AlertDescription>
          </Alert>
        )
      }
    >
      {printers.error ? (
        <QueryState
          isLoading={false}
          error={printers.error}
          onRetry={() => void printers.refetch()}
        />
      ) : (
        <PrintersCard
          establishmentId={establishmentId}
          printers={list}
          loading={printers.isPending}
          available={available}
          canConfigure={canConfigure}
          onAdd={() => setEditing('new')}
          onEdit={setEditing}
        />
      )}

      {available && routes.data && (
        <RoutingCard
          // Remount on a new ETag so the form starts from the saved set.
          key={routes.data.etag ?? 'routes'}
          establishmentId={establishmentId}
          routes={routes.data}
          printers={list}
          registers={registers.data ?? []}
          readOnly={!canConfigure}
          onReload={() => void routes.refetch()}
        />
      )}
      {available && routes.error && (
        <QueryState isLoading={false} error={routes.error} onRetry={() => void routes.refetch()} />
      )}

      {available && <JobsCard establishmentId={establishmentId} canRetry />}

      <FormSheet
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
        title={editing !== null && editing !== 'new' ? t('form.editTitle') : t('form.addTitle')}
        description={t('form.description')}
      >
        {editing !== null && (
          <PrinterForm
            key={editing === 'new' ? 'new' : editing.id}
            establishmentId={establishmentId}
            item={editing === 'new' ? undefined : editing}
            onDone={() => setEditing(null)}
          />
        )}
      </FormSheet>
    </Page>
  );
}

function PrintersCard({
  establishmentId,
  printers,
  loading,
  available,
  canConfigure,
  onAdd,
  onEdit,
}: {
  establishmentId: string;
  printers: Printer[];
  loading: boolean;
  available: boolean;
  canConfigure: boolean;
  onAdd: () => void;
  onEdit: (p: Printer) => void;
}) {
  const { t } = useTranslation('printers');
  const fmt = useInstantFormat();
  const check = useCheckPrinter(establishmentId);
  const test = useTestPrinter(establishmentId);
  const remove = useDeletePrinter(establishmentId);
  const problemText = usePrintingProblemText();

  const runCheck = (p: Printer) =>
    check.mutate(p.id, {
      onSuccess: (r) =>
        r.status === 'reachable'
          ? toast.success(t('check.reachable', { name: p.name }), {
              description:
                r.latency_ms !== undefined ? t('check.latency', { ms: r.latency_ms }) : undefined,
            })
          : toast.error(t('check.unreachable', { name: p.name }), { description: r.error }),
      onError: (err) => toast.error(problemText(err)),
    });

  const runTest = (p: Printer) =>
    test.mutate(p.id, {
      onSuccess: () => toast.success(t('test.queued', { name: p.name })),
      onError: (err) => toast.error(problemText(err)),
    });

  const row = (p: Printer) => {
    const checking = check.isPending && check.variables === p.id;
    const testing = test.isPending && test.variables === p.id;
    return (
      <li
        key={p.id}
        aria-label={p.name}
        className="flex flex-col gap-3 px-6 py-4 md:flex-row md:items-center md:gap-6"
      >
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <PrinterIcon aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />
          <div className="flex min-w-0 flex-col">
            <span className="flex flex-wrap items-center gap-1.5 font-medium">
              {p.name}
              {!p.active && <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>}
            </span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {printerAddress(p)}
            </span>
            <span className="text-xs text-muted-foreground">
              {t(`paper.${p.paper_width_mm}`)} · {t(`codepage.${p.codepage}`)}
            </span>
          </div>
        </div>
        <div className="flex min-w-0 flex-col items-start gap-1 md:w-56">
          <StatusBadge tone={PRINTER_STATUS_TONE[p.status]}>{t(`status.${p.status}`)}</StatusBadge>
          {p.last_checked_at && (
            <span className="text-xs text-muted-foreground">
              {t('status.checkedAt', { time: fmt.dateTime(p.last_checked_at) })}
            </span>
          )}
          {p.status === 'unreachable' && p.last_error && (
            <span
              className="max-w-full truncate text-xs text-muted-foreground"
              title={p.last_error}
            >
              {p.last_error}
            </span>
          )}
        </div>
        {available && (
          <div className="flex flex-wrap items-center gap-1 md:justify-end">
            <Button
              variant="ghost"
              size="sm"
              disabled={checking}
              aria-label={t('check.aria', { name: p.name })}
              onClick={() => runCheck(p)}
            >
              {checking ? <Spinner /> : <RadarIcon aria-hidden="true" />}
              {t('check.action')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={testing || !p.active}
              aria-label={t('test.aria', { name: p.name })}
              onClick={() => runTest(p)}
            >
              {testing ? <Spinner /> : <ReceiptTextIcon aria-hidden="true" />}
              {t('test.action')}
            </Button>
            {canConfigure && (
              <span className="flex items-center gap-1">
                <EditButton label={t('editAria', { name: p.name })} onClick={() => onEdit(p)} />
                <DeleteButton
                  label={t('delete.aria', { name: p.name })}
                  message={t('delete.message', { name: p.name })}
                  onConfirm={() => remove.mutateAsync({ id: p.id, version: p.version })}
                />
              </span>
            )}
          </div>
        )}
      </li>
    );
  };

  return (
    <SectionCard
      title={t('list.title')}
      titleId="printers-list-title"
      icon={PrinterIcon}
      description={t('list.description')}
      flush
    >
      {loading ? (
        <div className="px-6 pb-6">
          <QueryState isLoading error={null} rows={2} />
        </div>
      ) : printers.length === 0 ? (
        <EmptyState
          icon={PrinterIcon}
          title={t('list.empty')}
          description={available ? t('list.emptyHint') : t('list.emptyServer')}
          bordered={false}
          action={
            canConfigure ? (
              <Button variant="outline" onClick={onAdd}>
                <PlusIcon aria-hidden="true" />
                {t('add')}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <ul aria-labelledby="printers-list-title" className="divide-y border-t">
          {printers.map(row)}
        </ul>
      )}
    </SectionCard>
  );
}
