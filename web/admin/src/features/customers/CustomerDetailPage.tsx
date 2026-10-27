import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  BackLink,
  DataTable,
  EmptyState,
  Page,
  QueryState,
  SectionCard,
  StatCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  CalendarIcon,
  DownloadIcon,
  PencilIcon,
  PlusIcon,
  TicketIcon,
  UserRoundXIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import { STATUS_TONE, useOperationalPermissions } from '../bookings/model';
import { describeProblem } from '../bookings/problem';
import { NewBookingSheet } from '../bookings/NewBookingSheet';
import { CustomerCreditsSection } from '../credits';
import { downloadJson, useCustomer, useExportCustomer, type CustomerDetail } from './api';
import { CustomerAgreementCard } from './agreement/CustomerAgreementCard';
import { AnonymizeDialog, CustomerFormDialog } from './CustomerDialogs';

type RecentBooking = CustomerDetail['recent_bookings'][number];
type CustomerPass = CustomerDetail['season_passes'][number];

/** Scheda cliente: contacts, tags, consent, stats, credits, bookings and passes, GDPR actions. */
export function CustomerDetailPage() {
  const { t } = useTranslation('customers');
  const fmt = useInstantFormat();
  const establishmentId = useRequiredEstablishmentId();
  const navigate = useNavigate();
  const { customerId } = useParams({ strict: false }) as { customerId?: string };
  const query = useCustomer(establishmentId, customerId);
  const exportData = useExportCustomer(establishmentId);
  const perms = useOperationalPermissions();
  const [editing, setEditing] = useState(false);
  const [anonymizing, setAnonymizing] = useState(false);
  const [booking, setBooking] = useState(false);

  const back = (
    <BackLink>
      <Link to="/e/$establishmentId/clienti" params={{ establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('detail.back')}
      </Link>
    </BackLink>
  );
  if (!query.data) {
    return (
      <Page eyebrow={back} title={t('detail.title')}>
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={5}
        />
      </Page>
    );
  }
  const { customer: c, stats, recent_bookings, season_passes } = query.data;
  const anonymized = !!c.anonymized_at;
  const openBooking = (id: string) =>
    void navigate({
      to: '/e/$establishmentId/prenotazioni/$bookingId',
      params: { establishmentId, bookingId: id },
    });

  const runExport = async () => {
    try {
      const data = await exportData.mutateAsync(c.id);
      downloadJson(data, `cliente-${c.id}.json`);
      toast.success(t('export.done'));
    } catch (err) {
      toast.error(describeProblem(err, t).title);
    }
  };

  const bookingLink = (id: string, text: ReactNode) => (
    <Link
      to="/e/$establishmentId/prenotazioni/$bookingId"
      params={{ establishmentId, bookingId: id }}
      className="font-medium underline-offset-4 hover:underline"
      onClick={(e) => e.stopPropagation()}
    >
      {text}
    </Link>
  );

  const bookingColumns: ColumnDef<RecentBooking>[] = [
    {
      id: 'code',
      header: t('bookings.code'),
      meta: { label: t('bookings.code') },
      cell: ({ row }) => bookingLink(row.original.id, row.original.code),
    },
    {
      id: 'period',
      header: t('bookings.period'),
      meta: { label: t('bookings.period') },
      cell: ({ row: { original: b } }) => (
        <span className="tabular-nums">
          {formatIsoDate(b.starts_on)}
          {b.ends_on !== b.starts_on && ` → ${formatIsoDate(b.ends_on)}`}
        </span>
      ),
    },
    {
      id: 'status',
      header: t('bookings.status'),
      meta: { label: t('bookings.status') },
      cell: ({ row: { original: b } }) => (
        <StatusBadge tone={STATUS_TONE[b.status]}>{t(`bookings:status.${b.status}`)}</StatusBadge>
      ),
    },
    {
      id: 'total',
      header: t('bookings.total'),
      meta: { label: t('bookings.total'), align: 'right' },
      cell: ({ row }) => formatCents(row.original.total_amount),
    },
  ];

  const passColumns: ColumnDef<CustomerPass>[] = [
    {
      id: 'pass',
      header: t('passes.columns.pass'),
      meta: { label: t('passes.columns.pass') },
      cell: ({ row: { original: p } }) => bookingLink(p.id, `${p.spot_label} · ${p.code}`),
    },
    {
      id: 'period',
      header: t('passes.columns.period'),
      meta: { label: t('passes.columns.period') },
      cell: ({ row: { original: p } }) => (
        <span className="tabular-nums">
          {formatIsoDate(p.starts_on)} → {formatIsoDate(p.ends_on)}
        </span>
      ),
    },
    {
      id: 'released',
      header: t('passes.columns.released'),
      meta: { label: t('passes.columns.released') },
      cell: ({ row: { original: p } }) =>
        p.released_days > 0 ? (
          <Badge variant="outline">{t('passes.released', { count: p.released_days })}</Badge>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'status',
      header: t('passes.columns.status'),
      meta: { label: t('passes.columns.status') },
      cell: ({ row: { original: p } }) => (
        <StatusBadge tone={STATUS_TONE[p.status]}>{t(`bookings:status.${p.status}`)}</StatusBadge>
      ),
    },
  ];

  return (
    <Page
      eyebrow={back}
      title={
        <span className="flex flex-wrap items-center gap-3">
          {c.display_name}
          {anonymized && <StatusBadge tone="neutral">{t('anonymized')}</StatusBadge>}
        </span>
      }
      description={[c.phone, c.email].filter(Boolean).join(' · ') || undefined}
      actions={
        perms.canWrite && (
          <>
            {!anonymized && (
              <Button variant="destructive" onClick={() => setAnonymizing(true)}>
                <UserRoundXIcon aria-hidden="true" />
                {t('anonymize.action')}
              </Button>
            )}
            <Button
              variant="outline"
              onClick={() => void runExport()}
              disabled={exportData.isPending}
            >
              <DownloadIcon aria-hidden="true" />
              {t('export.action')}
            </Button>
            {!anonymized && (
              <Button variant="outline" onClick={() => setEditing(true)}>
                <PencilIcon aria-hidden="true" />
                {t('common.actions.edit', { ns: 'translation' })}
              </Button>
            )}
            {!anonymized && (
              <Button onClick={() => setBooking(true)}>
                <PlusIcon aria-hidden="true" />
                {t('detail.newBooking')}
              </Button>
            )}
          </>
        )
      }
    >
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <StatCard label={t('stats.bookings')} value={stats.bookings_count} />
        <StatCard label={t('stats.spent')} value={formatCents(stats.total_spent)} />
        <StatCard
          label={t('stats.balance')}
          value={formatCents(stats.balance_due)}
          tone={stats.balance_due > 0 ? 'danger' : 'default'}
        />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-6">
          <SectionCard
            flush
            title={t('bookings.title')}
            description={
              stats.last_booking_on
                ? t('bookings.last', { date: formatIsoDate(stats.last_booking_on) })
                : undefined
            }
          >
            <DataTable
              embedded
              caption={t('bookings.title')}
              columns={bookingColumns}
              data={recent_bookings}
              getRowId={(b) => b.id}
              onRowClick={(b) => openBooking(b.id)}
              getRowLabel={(b) => b.code}
              columnToggle={false}
              pageSize={0}
              empty={
                <EmptyState icon={CalendarIcon} bordered={false} title={t('bookings.empty')} />
              }
            />
          </SectionCard>

          <SectionCard flush title={t('passes.title')}>
            <DataTable
              embedded
              caption={t('passes.title')}
              columns={passColumns}
              data={season_passes}
              getRowId={(p) => p.id}
              columnToggle={false}
              pageSize={0}
              empty={<EmptyState icon={TicketIcon} bordered={false} title={t('passes.empty')} />}
            />
          </SectionCard>

          <CustomerCreditsSection
            establishmentId={establishmentId}
            customerId={c.id}
            bookings={recent_bookings}
            readOnly={anonymized}
          />
        </div>

        <div className="min-w-0 space-y-6">
          <CustomerAgreementCard
            establishmentId={establishmentId}
            customer={c}
            canWrite={perms.canWrite && !anonymized}
          />
          <SectionCard title={t('detail.profile')}>
            <dl className="space-y-4 text-sm">
              <Field label={t('fields.locale')}>
                {t(`common.languages.${c.locale}`, { ns: 'translation', defaultValue: c.locale })}
              </Field>
              <Field label={t('fields.tags')}>
                {c.tags.length ? (
                  <span className="flex flex-wrap gap-1">
                    {c.tags.map((tg) => (
                      <Badge key={tg} variant="outline">
                        {tg}
                      </Badge>
                    ))}
                  </span>
                ) : (
                  '—'
                )}
              </Field>
              <Field label={t('fields.marketing')}>
                {c.marketing_consent
                  ? t('consent.given', {
                      at: fmt.date(c.marketing_consent.given_at),
                      source: c.marketing_consent.source,
                    })
                  : t('consent.no')}
              </Field>
              <Field label={t('fields.notes')}>
                <span className="whitespace-pre-line">{c.notes || '—'}</span>
              </Field>
              {c.anonymized_at && (
                <Field label={t('anonymized')}>{fmt.dateTime(c.anonymized_at)}</Field>
              )}
            </dl>
          </SectionCard>
        </div>
      </div>

      <CustomerFormDialog
        establishmentId={establishmentId}
        customer={c}
        open={editing}
        onOpenChange={setEditing}
        onReload={() => void query.refetch()}
      />
      <AnonymizeDialog
        establishmentId={establishmentId}
        customer={c}
        open={anonymizing}
        onOpenChange={setAnonymizing}
        onReload={() => void query.refetch()}
      />
      <NewBookingSheet
        establishmentId={establishmentId}
        open={booking}
        onOpenChange={setBooking}
        defaults={{ customer: c }}
        onCreated={openBooking}
      />
    </Page>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="font-medium">{label}</dt>
      <dd className="text-muted-foreground">{children}</dd>
    </div>
  );
}
