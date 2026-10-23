import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { PageHeader, QueryState, StatusBadge } from '@arenile/shared/ui/patterns';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import { ArrowLeftIcon, DownloadIcon, PencilIcon, PlusIcon, UserRoundXIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import { STATUS_TONE, useOperationalPermissions } from '../bookings/model';
import { describeProblem } from '../bookings/problem';
import { NewBookingSheet } from '../bookings/NewBookingSheet';
import { downloadJson, useCustomer, useExportCustomer } from './api';
import { AnonymizeDialog, CustomerFormDialog } from './CustomerDialogs';

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
    <Button variant="ghost" size="sm" asChild>
      <Link to="/e/$establishmentId/clienti" params={{ establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('detail.back')}
      </Link>
    </Button>
  );
  if (!query.data) {
    return (
      <section aria-labelledby="page-title" className="space-y-6">
        {back}
        <PageHeader title={t('detail.title')} />
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={5}
        />
      </section>
    );
  }
  const {
    customer: c,
    stats,
    credits,
    credit_balance,
    recent_bookings,
    season_passes,
  } = query.data;
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

  return (
    <section aria-labelledby="page-title" className="space-y-6">
      {back}
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            {c.display_name}
            {anonymized && <StatusBadge tone="neutral">{t('anonymized')}</StatusBadge>}
          </span>
        }
        description={[c.phone, c.email].filter(Boolean).join(' · ') || undefined}
        actions={
          perms.canWrite && (
            <div className="flex flex-wrap gap-2">
              {!anonymized && (
                <Button onClick={() => setBooking(true)}>
                  <PlusIcon aria-hidden="true" />
                  {t('detail.newBooking')}
                </Button>
              )}
              {!anonymized && (
                <Button variant="outline" onClick={() => setEditing(true)}>
                  <PencilIcon aria-hidden="true" />
                  {t('common.actions.edit', { ns: 'translation' })}
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
                <Button variant="destructive" onClick={() => setAnonymizing(true)}>
                  <UserRoundXIcon aria-hidden="true" />
                  {t('anonymize.action')}
                </Button>
              )}
            </div>
          )
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label={t('stats.bookings')}>{stats.bookings_count}</Stat>
        <Stat label={t('stats.spent')}>{formatCents(stats.total_spent)}</Stat>
        <Stat label={t('stats.balance')} emphasis={stats.balance_due > 0}>
          {formatCents(stats.balance_due)}
        </Stat>
        <Stat label={t('stats.credit')}>{formatCents(credit_balance)}</Stat>
      </div>

      <div className="grid items-start gap-6 xl:grid-cols-3">
        <div className="space-y-6 xl:col-span-2">
          <Card className="gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="text-base">{t('bookings.title')}</CardTitle>
              {stats.last_booking_on && (
                <CardDescription>
                  {t('bookings.last', { date: formatIsoDate(stats.last_booking_on) })}
                </CardDescription>
              )}
            </CardHeader>
            {recent_bookings.length === 0 ? (
              <p className="px-6 py-6 text-sm text-muted-foreground">{t('bookings.empty')}</p>
            ) : (
              <Table>
                <caption className="sr-only">{t('bookings.title')}</caption>
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-6">{t('bookings.code')}</TableHead>
                    <TableHead>{t('bookings.period')}</TableHead>
                    <TableHead>{t('bookings.status')}</TableHead>
                    <TableHead className="pr-6 text-right">{t('bookings.total')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recent_bookings.map((b) => (
                    <TableRow
                      key={b.id}
                      className="cursor-pointer"
                      onClick={() => openBooking(b.id)}
                    >
                      <TableCell className="pl-6 font-medium">
                        <Link
                          to="/e/$establishmentId/prenotazioni/$bookingId"
                          params={{ establishmentId, bookingId: b.id }}
                          className="underline-offset-4 hover:underline"
                          onClick={(e) => e.stopPropagation()}
                        >
                          {b.code}
                        </Link>
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {formatIsoDate(b.starts_on)}
                        {b.ends_on !== b.starts_on && ` → ${formatIsoDate(b.ends_on)}`}
                      </TableCell>
                      <TableCell>
                        <StatusBadge tone={STATUS_TONE[b.status]}>
                          {t(`bookings:status.${b.status}`)}
                        </StatusBadge>
                      </TableCell>
                      <TableCell className="pr-6 text-right tabular-nums">
                        {formatCents(b.total_amount)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Card>

          <Card className="gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="text-base">{t('passes.title')}</CardTitle>
            </CardHeader>
            {season_passes.length === 0 ? (
              <p className="px-6 py-6 text-sm text-muted-foreground">{t('passes.empty')}</p>
            ) : (
              <ul className="divide-y">
                {season_passes.map((p) => (
                  <li
                    key={p.id}
                    className="flex flex-wrap items-center justify-between gap-2 px-6 py-3 text-sm"
                  >
                    <Link
                      to="/e/$establishmentId/prenotazioni/$bookingId"
                      params={{ establishmentId, bookingId: p.id }}
                      className="font-medium underline-offset-4 hover:underline"
                    >
                      {p.spot_label} · {p.code}
                    </Link>
                    <span className="text-muted-foreground tabular-nums">
                      {formatIsoDate(p.starts_on)} → {formatIsoDate(p.ends_on)}
                    </span>
                    <span className="flex items-center gap-2">
                      {p.released_days > 0 && (
                        <Badge variant="outline">
                          {t('passes.released', { count: p.released_days })}
                        </Badge>
                      )}
                      <StatusBadge tone={STATUS_TONE[p.status]}>
                        {t(`bookings:status.${p.status}`)}
                      </StatusBadge>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t('detail.profile')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 text-sm">
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
            </CardContent>
          </Card>
          <Card className="gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="text-base">{t('credits.title')}</CardTitle>
              <CardDescription>
                {t('credits.balance', { amount: formatCents(credit_balance) })}
              </CardDescription>
            </CardHeader>
            {credits.length === 0 ? (
              <p className="px-6 py-6 text-sm text-muted-foreground">{t('credits.empty')}</p>
            ) : (
              <ul className="divide-y text-sm">
                {credits.map((cr) => (
                  <li key={cr.id} className="flex items-center justify-between gap-2 px-6 py-3">
                    <span>
                      {t(`credits.source.${cr.source}`)}
                      <span className="block text-xs text-muted-foreground">
                        {fmt.date(cr.created_at)}
                        {cr.expires_on &&
                          ` · ${t('credits.expires', { date: formatIsoDate(cr.expires_on) })}`}
                      </span>
                    </span>
                    <span
                      className={
                        cr.amount < 0
                          ? 'text-muted-foreground tabular-nums'
                          : 'font-medium tabular-nums'
                      }
                    >
                      {cr.amount < 0 ? `−${formatCents(-cr.amount)}` : formatCents(cr.amount)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Card>
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
    </section>
  );
}

function Stat({
  label,
  children,
  emphasis,
}: {
  label: string;
  children: ReactNode;
  emphasis?: boolean;
}) {
  return (
    <Card className="gap-1 py-4">
      <CardContent className="px-4">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p
          className={
            emphasis
              ? 'text-xl font-semibold text-destructive tabular-nums'
              : 'text-xl font-semibold tabular-nums'
          }
        >
          {children}
        </p>
      </CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="font-medium">{label}</p>
      <div className="text-muted-foreground">{children}</div>
    </div>
  );
}
