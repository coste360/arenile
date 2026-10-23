import { dateFnsLocale } from '@arenile/shared/lib/dateLocales';
import { dateToIso, isoToDate, maxIso } from '@arenile/shared/lib/dates';
import { Button } from '@arenile/shared/ui/button';
import { Calendar } from '@arenile/shared/ui/calendar';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useManageRelease, type ManageBooking, type ManageReleaseResult } from '../api/public';
import { useBookingEnv } from '../flow/context';
import { formatDayShort, formatMoney } from '../flow/format';
import { problemMessageKey } from '../flow/problems';
import { PUBLIC_NS } from '../i18n';

/**
 * Season pass holders release single days (spec 7.7, 12.4): pick the days
 * on a calendar limited to the remaining season, preview the credit, apply.
 */
export function ReleaseSection({
  slug,
  token,
  booking,
}: {
  slug: string;
  token: string;
  booking: ManageBooking;
}) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { today } = useBookingEnv();
  const release = useManageRelease(slug, token);
  const [days, setDays] = useState<Date[]>([]);
  const [preview, setPreview] = useState<ManageReleaseResult | null>(null);
  const s = booking.summary;
  const money = (c: number) => formatMoney(c, s.currency, i18n.language);
  const from = isoToDate(maxIso(today, s.starts_on));
  const to = isoToDate(s.ends_on);
  const released = new Set((booking.releases ?? []).map((r) => r.date));
  const body = (dryRun: boolean) => ({
    dry_run: dryRun,
    days: days.map((d) => ({ date: dateToIso(d), slots: s.slots })),
  });

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('manage.releaseHint')}</p>
      <Calendar
        mode="multiple"
        selected={days}
        onSelect={(d) => {
          setDays(d ?? []);
          setPreview(null);
        }}
        locale={dateFnsLocale(i18n.language)}
        weekStartsOn={1}
        startMonth={from}
        endMonth={to}
        defaultMonth={from}
        disabled={[
          ...(from ? [{ before: from }] : []),
          ...(to ? [{ after: to }] : []),
          (d: Date) => released.has(dateToIso(d)),
        ]}
        className="rounded-lg border"
      />
      {preview && (
        <p className="text-sm font-medium" aria-live="polite">
          {t('manage.releaseCredit', { amount: money(preview.credit_amount) })}
        </p>
      )}
      {release.data?.applied && (
        <p role="status" className="text-sm font-medium">
          {t('manage.released', { amount: money(release.data.credit_amount) })}
        </p>
      )}
      {release.error && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemMessageKey(release.error))}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          className="h-11"
          disabled={days.length === 0 || release.isPending}
          onClick={() => release.mutate(body(true), { onSuccess: (r) => setPreview(r) })}
        >
          {release.isPending && <Spinner />}
          {t('manage.releasePreview')}
        </Button>
        {preview && (
          <Button
            className="h-11"
            disabled={release.isPending}
            onClick={() =>
              release.mutate(body(false), {
                onSuccess: () => {
                  setDays([]);
                  setPreview(null);
                },
              })
            }
          >
            {t('manage.releaseApply')}
          </Button>
        )}
      </div>
      {(booking.releases ?? []).length > 0 && (
        <div className="space-y-1">
          <p className="text-sm font-medium">{t('manage.releasedDays')}</p>
          <ul className="flex flex-wrap gap-1.5 text-sm">
            {(booking.releases ?? []).map((r) => (
              <li key={`${r.date}-${r.slot}`} className="rounded-md border px-2 py-1">
                {formatDayShort(r.date, i18n.language)} · {money(r.credit_amount)}
                {r.resold ? ` · ${t('manage.resold')}` : ''}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
