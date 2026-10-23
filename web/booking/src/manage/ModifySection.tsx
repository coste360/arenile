import { modifySelection } from './modify';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useManageModify, type ManageBooking, type ManageModifyResult } from '../api/public';
import { useBookingEnv } from '../flow/context';
import { formatMoney } from '../flow/format';
import { problemMessageKey } from '../flow/problems';
import { bookableWindow, compatibleFormulas, type StayDates } from '../flow/selection';
import { PUBLIC_NS } from '../i18n';
import { AvailabilityRangePicker } from '../steps/when/AvailabilityRangePicker';

/** Change of dates within the policy limits, previewed (dry run) before applying (spec 12.4, 10.3). */
export function ModifySection({
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
  const { online, establishment, today } = useBookingEnv();
  const id = useId();
  const modify = useManageModify(slug, token);
  const s = booking.summary;
  const [dates, setDates] = useState<StayDates | undefined>({ from: s.starts_on, to: s.ends_on });
  const [preview, setPreview] = useState<ManageModifyResult | null>(null);
  const window = bookableWindow(today, online, establishment.season);
  const formula =
    compatibleFormulas(dates, s.slots, online.formulas, establishment.season)[0] ?? s.formula;
  const money = (c: number) => formatMoney(c, s.currency, i18n.language);
  const complete = !!dates?.to;

  const run = async (dryRun: boolean) => {
    if (!dates?.to) return;
    try {
      const r = await modify.mutateAsync({
        version: booking.version,
        body: {
          ...modifySelection(booking, { from: dates.from, to: dates.to }, formula),
          dry_run: dryRun,
        },
      });
      setPreview(dryRun ? r : null);
    } catch {
      setPreview(null);
    }
  };

  if (modify.data?.applied) {
    return (
      <p role="status" className="text-sm font-medium">
        {t('manage.modifyApplied')}
      </p>
    );
  }

  return (
    <div className="space-y-4">
      {booking.allowed.modify_until && (
        <p className="text-sm text-muted-foreground">
          {t('manage.modifyHint', { date: formatIsoDate(booking.allowed.modify_until) })}
        </p>
      )}
      <div className="space-y-2">
        <p id={`${id}-label`} className="text-sm font-medium">
          {tc('booking.when.dates')}
        </p>
        <AvailabilityRangePicker
          id={`${id}-dates`}
          slug={slug}
          value={dates}
          min={window.min}
          max={window.max}
          slots={s.slots}
          showRatePeriods={false}
          onChange={(d) => {
            setDates(d);
            setPreview(null);
          }}
          aria-labelledby={`${id}-label ${id}-dates`}
        />
      </div>
      {preview && (
        <div className="space-y-1 rounded-lg border bg-muted/30 p-3 text-sm" aria-live="polite">
          {preview.conflicts.length > 0 || preview.violations.length > 0 ? (
            <p className="text-destructive">{t('manage.modifyConflict')}</p>
          ) : (
            <>
              <p className="font-medium">
                {t('manage.modifyNewTotal', { amount: money(preview.new_total) })}
              </p>
              <p>
                {preview.difference >= 0
                  ? t('manage.modifyDifferencePay', { amount: money(preview.difference) })
                  : t('manage.modifyDifferenceRefund', { amount: money(-preview.difference) })}
              </p>
              {!!preview.fee && (
                <p className="text-muted-foreground">
                  {t('manage.modifyFee', { amount: money(preview.fee) })}
                </p>
              )}
            </>
          )}
        </div>
      )}
      {modify.error && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemMessageKey(modify.error))}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          className="h-11"
          disabled={!complete || modify.isPending}
          onClick={() => void run(true)}
        >
          {modify.isPending && <Spinner />}
          {t('manage.modifyPreview')}
        </Button>
        {preview && preview.conflicts.length === 0 && preview.violations.length === 0 && (
          <Button className="h-11" disabled={modify.isPending} onClick={() => void run(false)}>
            {t('manage.modifyApply')}
          </Button>
        )}
      </div>
    </div>
  );
}
