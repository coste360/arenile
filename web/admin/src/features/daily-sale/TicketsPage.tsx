import { useMe } from '@arenile/shared/api';
import { formatInstantTime } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import {
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  EmptyState,
  Page,
  QueryState,
  RowActions,
  SectionCard,
  SegmentedControl,
  StatCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Tooltip, TooltipContent, TooltipTrigger } from '@arenile/shared/ui/tooltip';
import { Link, useSearch } from '@tanstack/react-router';
import {
  ArrowRightLeftIcon,
  BanIcon,
  DownloadIcon,
  MapPinIcon,
  PrinterIcon,
  ReceiptIcon,
  Settings2Icon,
  TicketIcon,
  TicketXIcon,
  WalletIcon,
  ZapIcon,
} from 'lucide-react';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';
import { localize } from '../configuration/lib/i18nText';
import { isIsoDate, todayIn } from '../operations/lib/dates';
import { grantedPermissions } from '../operations/lib/permissions';
import { useDailySaleSettings, useTickets, type Ticket, type TicketState } from './api';
import { STATE_TONE, ticketsCsvUrl, vatRateLabel } from './lib';
import { MoveTicketDialog, VoidTicketDialog } from './TicketDialogs';
import { TicketPrintDialog } from './TicketPrintDialog';
import { TicketSettingsDialog } from './TicketSettingsDialog';

type StateFilter = 'all' | TicketState;
const FILTERS: readonly StateFilter[] = ['all', 'assigned', 'unassigned', 'entered', 'voided'];

function IconAction({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="icon"
          variant="ghost"
          aria-label={label}
          onClick={onClick}
          disabled={disabled}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * "Ticket del giorno" (spec 14.3): the day's tickets with number, time,
 * spot or zone, state, amount and method; summary by method and VAT rate
 * (the same totals as the cash session and the corrispettivi), CSV export,
 * and per ticket reprint, move/assign and void.
 */
export function TicketsPage() {
  const { t, i18n } = useTranslation('dailySale');
  const locale = i18n.language || 'it';
  const establishmentId = useRequiredEstablishmentId();
  const { establishment } = useCurrentEstablishment();
  const me = useMe();
  const granted = grantedPermissions(me.data, establishment);
  const canWrite = granted.has('bookings.write');
  const canSell = canWrite && granted.has('cash.operate');
  const canConfigure = granted.has('establishment.configure');
  const settings = useDailySaleSettings(establishmentId);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const timeZone = establishment?.timezone ?? 'Europe/Rome';
  // `?date=` opens another day (links from the map or Oggi); today by default.
  const search = useSearch({ strict: false }) as { date?: unknown };
  const [pickedDate, setDate] = useState<string | undefined>();
  const date =
    pickedDate ?? (isIsoDate(search.date) ? search.date : todayIn(establishment?.timezone));
  const [filter, setFilter] = useState<StateFilter>('all');
  const tickets = useTickets(establishmentId, {
    date,
    ...(filter !== 'all' ? { state: filter } : {}),
  });
  const [printing, setPrinting] = useState<Ticket | null>(null);
  const [moving, setMoving] = useState<Ticket | null>(null);
  const [voiding, setVoiding] = useState<Ticket | null>(null);
  const summary = tickets.data?.summary;

  const columns = useMemo<ColumnDef<Ticket>[]>(() => {
    const name = (text: Record<string, string> | undefined) => localize(text, locale, 'it');
    return [
      {
        id: 'number',
        accessorKey: 'ticket_number',
        meta: { label: t('tickets.col.number') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('tickets.col.number')} />
        ),
        cell: ({ row }) => (
          <span className="flex flex-col items-start gap-1">
            <span className="font-semibold tabular-nums">{row.original.number_label}</span>
            {/* Phones: the state column scrolls off-screen, so it rides with the number. */}
            <StatusBadge tone={STATE_TONE[row.original.state]} className="sm:hidden">
              {t(`state.${row.original.state}`)}
            </StatusBadge>
          </span>
        ),
      },
      {
        id: 'time',
        accessorKey: 'created_at',
        meta: { label: t('tickets.col.time') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('tickets.col.time')} />
        ),
        cell: ({ row }) => (
          <span className="tabular-nums">
            {formatInstantTime(row.original.created_at, { locale, timeZone })}
          </span>
        ),
      },
      {
        id: 'where',
        accessorFn: (r) => r.spot_label ?? name(r.zone_name),
        meta: { label: t('tickets.col.where') },
        header: t('tickets.col.where'),
        cell: ({ row }) => {
          const r = row.original;
          if (r.kind === 'entry') return <span>{t('tickets.entry')}</span>;
          return r.spot_label ? (
            <span>
              <span className="font-medium">{r.spot_label}</span>{' '}
              <span className="text-muted-foreground">{name(r.zone_name)}</span>
            </span>
          ) : (
            <span>{name(r.zone_name)}</span>
          );
        },
      },
      {
        id: 'slots',
        accessorKey: 'slots',
        meta: { label: t('tickets.col.slots') },
        header: t('tickets.col.slots'),
        cell: ({ row }) => t(`slots.${row.original.slots}`),
      },
      {
        id: 'holder',
        accessorFn: (r) => r.holder_name ?? '',
        meta: { label: t('tickets.col.holder') },
        header: t('tickets.col.holder'),
      },
      {
        id: 'state',
        accessorKey: 'state',
        meta: { label: t('tickets.col.state') },
        header: t('tickets.col.state'),
        cell: ({ row }) => (
          <StatusBadge tone={STATE_TONE[row.original.state]}>
            {t(`state.${row.original.state}`)}
          </StatusBadge>
        ),
      },
      {
        id: 'amount',
        accessorKey: 'total_amount',
        meta: { label: t('tickets.col.amount'), align: 'right' },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('tickets.col.amount')} />
        ),
        cell: ({ row }) => (
          <span
            className={
              row.original.status === 'voided' ? 'text-muted-foreground line-through' : undefined
            }
          >
            {formatCents(row.original.total_amount)}
          </span>
        ),
      },
      {
        id: 'method',
        accessorKey: 'payment_method',
        meta: { label: t('tickets.col.method') },
        header: t('tickets.col.method'),
        cell: ({ row }) => t(`method.${row.original.payment_method}`),
      },
      {
        id: 'actions',
        enableHiding: false,
        header: () => <span className="sr-only">{t('tickets.col.actions')}</span>,
        cell: ({ row }) => {
          const r = row.original;
          const voided = r.status === 'voided';
          return (
            <RowActions>
              <IconAction
                label={t('tickets.printAria', { number: r.number_label })}
                onClick={() => setPrinting(r)}
              >
                <PrinterIcon aria-hidden="true" />
              </IconAction>
              {canWrite && r.kind === 'sale' && (
                <IconAction
                  label={t(r.needs_assignment ? 'tickets.assignAria' : 'tickets.moveAria', {
                    number: r.number_label,
                  })}
                  onClick={() => setMoving(r)}
                  disabled={voided}
                >
                  {r.needs_assignment ? (
                    <MapPinIcon aria-hidden="true" />
                  ) : (
                    <ArrowRightLeftIcon aria-hidden="true" />
                  )}
                </IconAction>
              )}
              {canWrite && (
                <IconAction
                  label={t('tickets.voidAria', { number: r.number_label })}
                  onClick={() => setVoiding(r)}
                  disabled={voided || r.state === 'entered'}
                >
                  <BanIcon aria-hidden="true" />
                </IconAction>
              )}
            </RowActions>
          );
        },
      },
    ];
  }, [t, locale, timeZone, canWrite]);

  const cash = summary?.by_method.find((m) => m.method === 'cash');
  const card = summary?.by_method.find((m) => m.method === 'card');
  const sellLink = canSell && (
    <Button asChild>
      <Link to="/e/$establishmentId/vendita-rapida" params={{ establishmentId }}>
        <ZapIcon aria-hidden="true" />
        {t('sale.title')}
      </Link>
    </Button>
  );

  return (
    <Page
      title={t('tickets.title')}
      description={t('tickets.description')}
      actions={
        <>
          <Button variant="outline" asChild>
            <a
              href={ticketsCsvUrl(establishmentId, date)}
              download={`arenile_ticket_${date}.csv`}
              aria-label={t('tickets.csvAria', { date })}
            >
              <DownloadIcon aria-hidden="true" />
              {t('tickets.csv')}
            </a>
          </Button>
          {canConfigure && settings.data && (
            <Button variant="outline" onClick={() => setSettingsOpen(true)}>
              <Settings2Icon aria-hidden="true" />
              {t('settings.open')}
            </Button>
          )}
          {sellLink}
        </>
      }
    >
      {settingsOpen && settings.data && (
        <TicketSettingsDialog
          establishmentId={establishmentId}
          settings={settings.data}
          onClose={() => setSettingsOpen(false)}
        />
      )}
      <div className="flex flex-wrap items-center gap-3">
        <div className="w-full sm:w-48">
          <DatePicker
            aria-label={t('tickets.date')}
            value={date}
            onChange={(d) => d && setDate(d)}
          />
        </div>
        <SegmentedControl
          label={t('tickets.filter')}
          value={filter}
          onValueChange={(v) => setFilter(v as StateFilter)}
          options={FILTERS.map((f) => ({
            value: f,
            label: f === 'all' ? t('tickets.all') : t(`state.${f}`),
          }))}
          collapseOnNarrow
        />
      </div>

      {summary && (
        <section
          aria-label={t('tickets.summary')}
          className="grid grid-cols-2 gap-4 lg:grid-cols-4"
        >
          <StatCard label={t('tickets.issued')} value={summary.issued} icon={TicketIcon} />
          <StatCard
            label={t('tickets.takings')}
            value={formatCents(summary.total_amount)}
            icon={WalletIcon}
            hint={t('tickets.byMethod', {
              cash: formatCents(cash?.amount ?? 0),
              card: formatCents(card?.amount ?? 0),
            })}
          />
          <StatCard
            label={t('state.unassigned')}
            value={summary.unassigned}
            icon={MapPinIcon}
            tone={summary.unassigned > 0 ? 'warning' : 'default'}
          />
          <StatCard label={t('tickets.voided')} value={summary.voided} icon={TicketXIcon} />
        </section>
      )}

      <SectionCard title={t('tickets.listTitle')} flush>
        <DataTable
          embedded
          caption={t('tickets.title')}
          columns={columns}
          data={tickets.data?.items ?? []}
          loading={tickets.isPending}
          error={
            tickets.error ? (
              <QueryState
                isLoading={false}
                error={tickets.error}
                onRetry={() => void tickets.refetch()}
              />
            ) : undefined
          }
          getRowId={(r) => r.id}
          initialSorting={[{ id: 'number', desc: true }]}
          searchable
          pageSize={50}
          empty={
            <EmptyState
              icon={ReceiptIcon}
              title={t('tickets.empty')}
              bordered={false}
              action={sellLink || undefined}
            />
          }
        />
      </SectionCard>

      {summary && summary.vat.length > 0 && (
        <SectionCard title={t('tickets.vatTitle')} description={t('tickets.vatDescription')}>
          <dl className="grid gap-2 text-sm sm:max-w-md">
            {summary.vat.map((v) => (
              <div key={v.rate} className="flex justify-between gap-4">
                <dt className="shrink-0 font-medium">
                  {t('print.vatLine', { rate: vatRateLabel(v.rate, locale) })}
                </dt>
                <dd className="text-right tabular-nums">
                  {t('print.vatAmounts', {
                    taxable: formatCents(v.taxable),
                    tax: formatCents(v.tax),
                  })}
                </dd>
              </div>
            ))}
          </dl>
        </SectionCard>
      )}

      {printing && (
        <TicketPrintDialog
          establishmentId={establishmentId}
          ticketId={printing.id}
          open
          onOpenChange={(o) => !o && setPrinting(null)}
          mode="reprint"
        />
      )}
      {moving && (
        <MoveTicketDialog
          establishmentId={establishmentId}
          ticket={moving}
          onOpenChange={(o) => !o && setMoving(null)}
        />
      )}
      {voiding && (
        <VoidTicketDialog
          establishmentId={establishmentId}
          ticket={voiding}
          onOpenChange={(o) => !o && setVoiding(null)}
        />
      )}
    </Page>
  );
}
