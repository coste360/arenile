import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { DateRangePicker } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { CalendarRangeIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { todayIso } from '../bookings/model';
import { describeProblem, type ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { useCreateReleases, type SeasonPass } from './api';
import { ISO_WEEKDAYS, rangeReleaseDates } from './releaseRange';

type Slots = SeasonPass['slots'];

export interface ReleaseRangeFormProps {
  establishmentId: string;
  pass: Pick<SeasonPass, 'id' | 'starts_on' | 'ends_on' | 'slots'>;
  /** Injected for tests; defaults to the browser's today. */
  today?: string;
  /** Called after a successful release (e.g. to close a dialog). */
  onDone?: () => void;
}

/**
 * Release of a season pass by range (spec 30.8): from-to and, optionally,
 * only some weekdays ("ogni lunedì di luglio"), in one request. The days
 * become bookable at once; the credit follows season_release_credit. The
 * count is an estimate: the server skips closed days and days already
 * released.
 */
export function ReleaseRangeForm({
  establishmentId,
  pass,
  today = todayIso(),
  onDone,
}: ReleaseRangeFormProps) {
  const { t, i18n } = useTranslation('passes');
  const create = useCreateReleases(establishmentId);
  const weekdaysId = useId();
  const slotsId = useId();
  const noteId = useId();
  const [range, setRange] = useState<{ from: string; to?: string } | undefined>();
  const [weekdays, setWeekdays] = useState<string[]>([]);
  const [slots, setSlots] = useState<Slots>(pass.slots);
  const [note, setNote] = useState('');
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const min = pass.starts_on > today ? pass.starts_on : today;
  const days = range
    ? rangeReleaseDates(range.from, range.to ?? range.from, weekdays.map(Number), pass, today)
    : [];
  const weekdayName = (n: number) =>
    // 2024-01-01 is a Monday: n-1 days after it is ISO weekday n.
    new Intl.DateTimeFormat(i18n.language, { weekday: 'short', timeZone: 'UTC' }).format(
      new Date(Date.UTC(2024, 0, n)),
    );
  const weekdayLong = (n: number) =>
    new Intl.DateTimeFormat(i18n.language, { weekday: 'long', timeZone: 'UTC' }).format(
      new Date(Date.UTC(2024, 0, n)),
    );

  const submit = async () => {
    if (!range) return;
    setFailure(null);
    try {
      await create.mutateAsync({
        passId: pass.id,
        body: {
          dates: [],
          from: range.from,
          to: range.to ?? range.from,
          ...(weekdays.length > 0 ? { weekdays: weekdays.map(Number).sort() } : {}),
          slots,
          ...(note.trim() ? { note: note.trim() } : {}),
        },
      });
      toast.success(t('releaseRange.done', { count: days.length }));
      setRange(undefined);
      setWeekdays([]);
      setNote('');
      onDone?.();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  };

  return (
    <div className="space-y-4" data-testid="release-range">
      <ProblemAlert problem={failure} />
      <div className="space-y-1.5">
        <Label>{t('releaseRange.period')}</Label>
        <DateRangePicker
          aria-label={t('releaseRange.period')}
          value={range ?? null}
          min={min}
          max={pass.ends_on}
          onChange={(v) => setRange(v ? { from: v.from, to: v.to } : undefined)}
          numberOfMonths={1}
        />
      </div>
      <div className="space-y-1.5">
        <p id={weekdaysId} className="text-sm font-medium">
          {t('releaseRange.weekdays')}
        </p>
        <ToggleGroup
          type="multiple"
          variant="outline"
          size="sm"
          spacing={1}
          aria-labelledby={weekdaysId}
          value={weekdays}
          onValueChange={setWeekdays}
          className="flex-wrap"
        >
          {ISO_WEEKDAYS.map((n) => (
            <ToggleGroupItem key={n} value={String(n)} aria-label={weekdayLong(n)}>
              {weekdayName(n)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <p className="text-xs text-muted-foreground">{t('releaseRange.weekdaysHint')}</p>
      </div>
      <div className="space-y-1.5">
        <p id={slotsId} className="text-sm font-medium">
          {t('releases.slots')}
        </p>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          aria-labelledby={slotsId}
          value={slots}
          onValueChange={(v) => v && setSlots(v as Slots)}
        >
          {(['FULL', 'AM', 'PM'] as const).map((s) => (
            <ToggleGroupItem key={s} value={s} disabled={pass.slots !== 'FULL' && s !== pass.slots}>
              {t(`bookings:slots.${s}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={noteId}>{t('releases.note')}</Label>
        <Input id={noteId} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
      </div>
      <div className="flex flex-wrap items-center justify-end gap-3">
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {range ? t('releaseRange.count', { count: days.length }) : ''}
        </p>
        <Button onClick={() => void submit()} disabled={days.length === 0 || create.isPending}>
          {create.isPending ? <Spinner /> : <CalendarRangeIcon aria-hidden="true" />}
          {t('releaseRange.submit')}
        </Button>
      </div>
    </div>
  );
}
