import { dateFnsLocale } from '@arenile/shared/lib/dateLocales';
import { dateToIso, formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import { Calendar } from '@arenile/shared/ui/calendar';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { ConfirmDialog, StatusBadge } from '@arenile/shared/ui/patterns';
import { Undo2Icon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { todayIso } from '../bookings/model';
import { describeProblem, type ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { useCreateReleases, useRevokeRelease, type SeasonPass, type SeasonRelease } from './api';
import {
  dayStatus,
  isReleasable,
  MAX_RELEASE_DATES,
  pruneSelection,
  releasedHalves,
  releasesCredit,
} from './releases';

type Slots = SeasonPass['slots'];

export interface ReleaseCalendarProps {
  establishmentId: string;
  pass: SeasonPass;
  releases: SeasonRelease[];
  canWrite: boolean;
  /** Injected for tests; defaults to the browser's today. */
  today?: string;
}

/**
 * Releases calendar of a season pass (spec 7.7): pick the days (and the half
 * of the day) the subscriber will not use; releasing makes the spot bookable
 * on them. Released days not yet resold can be revoked; resold ones accrue
 * credit for the subscriber according to the policy.
 */
export function ReleaseCalendar({
  establishmentId,
  pass,
  releases,
  canWrite,
  today = todayIso(),
}: ReleaseCalendarProps) {
  const { t, i18n } = useTranslation('passes');
  const create = useCreateReleases(establishmentId);
  const revoke = useRevokeRelease(establishmentId);
  const [slots, setSlots] = useState<Slots>(pass.slots);
  const [selection, setSelection] = useState<string[]>([]);
  const [note, setNote] = useState('');
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const released = useMemo(() => releasedHalves(releases), [releases]);
  const passWindow = { starts_on: pass.starts_on, ends_on: pass.ends_on, slots: pass.slots };
  const toDates = (pred: (r: SeasonRelease) => boolean) =>
    [...new Set(releases.filter(pred).map((r) => r.date))].map((d) => isoToDate(d) as Date);
  const firstMonth = isoToDate(
    pass.starts_on < today && pass.ends_on >= today ? today : pass.starts_on,
  );
  const active = [...releases]
    .filter((r) => r.status !== 'revoked')
    .sort((a, b) => a.date.localeCompare(b.date) || a.slot.localeCompare(b.slot));

  const submit = async () => {
    setFailure(null);
    try {
      await create.mutateAsync({
        passId: pass.id,
        body: { dates: selection, slots, ...(note.trim() ? { note: note.trim() } : {}) },
      });
      toast.success(t('releases.done', { count: selection.length }));
      setSelection([]);
      setNote('');
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm">
          {t('releases.summary', { count: pass.released_days })}
          {' · '}
          <span className="font-medium">
            {t('releases.credit', {
              amount: formatCents(Math.max(pass.credit_amount, releasesCredit(releases))),
            })}
          </span>
        </p>
        {canWrite && (
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            aria-label={t('releases.slots')}
            value={slots}
            onValueChange={(v) => {
              if (!v) return;
              setSlots(v as Slots);
              setSelection((s) => pruneSelection(s, v as Slots, passWindow, released, today));
            }}
          >
            {(['FULL', 'AM', 'PM'] as const).map((s) => (
              <ToggleGroupItem
                key={s}
                value={s}
                disabled={pass.slots !== 'FULL' && s !== pass.slots}
              >
                {t(`bookings:slots.${s}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        )}
      </div>
      <ProblemAlert problem={failure} />
      <Calendar
        mode="multiple"
        selected={selection.map((d) => isoToDate(d) as Date)}
        onSelect={(dates) =>
          setSelection(
            [...new Set((dates ?? []).map(dateToIso))].sort().slice(0, MAX_RELEASE_DATES),
          )
        }
        disabled={(day) =>
          !canWrite || !isReleasable(dateToIso(day), slots, passWindow, released, today)
        }
        defaultMonth={firstMonth}
        startMonth={isoToDate(pass.starts_on)}
        endMonth={isoToDate(pass.ends_on)}
        numberOfMonths={2}
        locale={dateFnsLocale(i18n.language)}
        weekStartsOn={1}
        modifiers={{
          released: toDates((r) => r.status === 'released'),
          resold: toDates((r) => r.status === 'resold'),
        }}
        modifiersClassNames={{
          released:
            '[&>button]:bg-highlight [&>button]:text-highlight-foreground [&>button]:opacity-100',
          resold: '[&>button]:bg-success/25 [&>button]:font-semibold [&>button]:opacity-100',
        }}
        className="rounded-lg border"
      />
      <ul
        className="flex flex-wrap gap-4 text-xs text-muted-foreground"
        aria-label={t('releases.legend')}
      >
        <li className="flex items-center gap-1.5">
          <span aria-hidden="true" className="size-3 rounded-sm bg-primary" />
          {t('releases.legendSelected')}
        </li>
        <li className="flex items-center gap-1.5">
          <span aria-hidden="true" className="size-3 rounded-sm bg-highlight" />
          {t('releases.legendReleased')}
        </li>
        <li className="flex items-center gap-1.5">
          <span aria-hidden="true" className="size-3 rounded-sm bg-success/25" />
          {t('releases.legendResold')}
        </li>
      </ul>
      {canWrite && (
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-48 flex-1 space-y-1.5">
            <Label htmlFor="release-note">{t('releases.note')}</Label>
            <Input
              id="release-note"
              value={note}
              maxLength={500}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <Button
            onClick={() => void submit()}
            disabled={selection.length === 0 || create.isPending}
          >
            {create.isPending && <Spinner />}
            {t('releases.submit', { count: selection.length })}
          </Button>
        </div>
      )}

      <section aria-label={t('releases.list')} className="space-y-2">
        <h3 className="text-sm font-medium">{t('releases.list')}</h3>
        {active.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('releases.empty')}</p>
        ) : (
          <ul className="divide-y rounded-lg border text-sm">
            {active.map((r) => (
              <li
                key={r.id}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
              >
                <span className="tabular-nums">
                  {formatIsoDate(r.date)} · {t(`bookings:slots.${r.slot}`)}
                </span>
                <span className="flex items-center gap-2">
                  <StatusBadge tone={r.status === 'resold' ? 'success' : 'highlight'}>
                    {t(`releases.status.${r.status}`)}
                  </StatusBadge>
                  {r.credit_amount > 0 && (
                    <span className="tabular-nums">{formatCents(r.credit_amount)}</span>
                  )}
                  {canWrite &&
                    r.status === 'released' &&
                    dayStatus(r.date, releases) !== 'resold' && (
                      <ConfirmDialog
                        title={t('releases.revokeTitle', { date: formatIsoDate(r.date) })}
                        description={t('releases.revokeText')}
                        confirmLabel={t('releases.revoke')}
                        errorText={(err) => describeProblem(err, t).title}
                        onConfirm={() => revoke.mutateAsync({ passId: pass.id, releaseId: r.id })}
                        trigger={
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={t('releases.revokeAria', {
                              date: formatIsoDate(r.date),
                              slot: t(`bookings:slots.${r.slot}`),
                            })}
                          >
                            <Undo2Icon aria-hidden="true" />
                            {t('releases.revoke')}
                          </Button>
                        }
                      />
                    )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
