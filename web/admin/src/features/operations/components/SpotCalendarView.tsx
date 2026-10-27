import { formatIsoDate } from '@arenile/shared/lib/dates';
import { cn } from '@arenile/shared/lib/utils';
import { STATUS_CLASSES, type SpotStatus } from '@arenile/shared/map';
import { QueryState } from '@arenile/shared/ui/patterns';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useSpotCalendar } from '../api/m8';
import {
  CALENDAR_LEGEND,
  calendarMonths,
  cellStatus,
  isDayFree,
  type SpotCalendar,
  type SpotCalendarBooking,
  type SpotCalendarDay,
} from '../lib/spotCalendar';
import { StatusGlyph } from './StatusGlyph';

export interface SpotCalendarViewProps {
  establishmentId: string;
  spotId: string;
  /** Load only while visible. */
  enabled?: boolean;
  /** Customer surnames in the day names (bookings.read). */
  canSeeCustomers: boolean;
  /** Click on a free day: new booking from that day (absent = read only). */
  onFreeDay?: (calendar: SpotCalendar, date: string) => void;
  /** Click on an occupied day: open the booking. */
  onBooking?: (bookingId: string) => void;
  /** Compact: no legend (hover preview). */
  compact?: boolean;
}

const DAY_COLUMNS = Array.from({ length: 31 }, (_, i) => i + 1);

/**
 * Season calendar of a spot (spec 30.5): one row per month, one column per
 * day; each day is split in a morning and an afternoon half with the status
 * colour and glyph (free, booked, season pass, released, held, blocked, to
 * collect). A free day starts a new booking, an occupied day opens its
 * booking. Every day is a button named with its date and statuses, so the
 * grid works with the keyboard and screen readers; it scrolls sideways
 * inside its frame on narrow screens.
 */
export function SpotCalendarView({
  establishmentId,
  spotId,
  enabled = true,
  canSeeCustomers,
  onFreeDay,
  onBooking,
  compact,
}: SpotCalendarViewProps) {
  const { t, i18n } = useTranslation('operations');
  const query = useSpotCalendar(establishmentId, spotId, enabled);
  const cal = query.data;
  const bookings = useMemo(
    () => new Map((cal?.bookings ?? []).map((b) => [b.booking_id, b])),
    [cal],
  );
  const months = useMemo(() => calendarMonths(cal?.days ?? []), [cal]);
  const monthName = (key: string) =>
    new Intl.DateTimeFormat(i18n.language, { month: 'short', timeZone: 'UTC' }).format(
      new Date(`${key}-01T00:00:00Z`),
    );

  if (!cal) {
    return (
      <QueryState
        isLoading={query.isLoading}
        error={query.error}
        onRetry={() => void query.refetch()}
        rows={4}
      />
    );
  }

  const describe = (d: SpotCalendarDay) => {
    const half = (slot: 'am' | 'pm') => {
      const cell = d[slot];
      const st = cellStatus(cell, bookings);
      const b = cell.booking_id ? bookings.get(cell.booking_id) : undefined;
      const who = b ? bookingName(b, canSeeCustomers) : '';
      return `${t(`slot.${slot === 'am' ? 'AM' : 'PM'}`)} ${t(`map.status.${st}`, { ns: 'translation' })}${who ? ` (${who})` : ''}`;
    };
    return `${formatIsoDate(d.date)}: ${half('am')}, ${half('pm')}`;
  };

  const activate = (d: SpotCalendarDay) => {
    if (isDayFree(d)) {
      onFreeDay?.(cal, d.date);
      return;
    }
    const id = d.am.booking_id ?? d.pm.booking_id;
    if (id) onBooking?.(id);
  };

  return (
    <div className="space-y-3" data-testid="spot-calendar">
      <div className="overflow-x-auto rounded-lg border">
        <table className="border-separate border-spacing-0.5 p-1 text-xs">
          <caption className="sr-only">{t('calendar.caption', { label: cal.label })}</caption>
          <thead>
            <tr>
              <th scope="col" className="sr-only">
                {t('calendar.month')}
              </th>
              {DAY_COLUMNS.map((n) => (
                <th
                  key={n}
                  scope="col"
                  className="w-6 text-center font-normal text-muted-foreground tabular-nums"
                >
                  {n}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {months.map((m) => (
              <tr key={m.key}>
                <th scope="row" className="pr-1 text-left font-medium whitespace-nowrap capitalize">
                  {monthName(m.key)}
                </th>
                {m.days.map((d, i) =>
                  d ? (
                    <td key={i} className="p-0">
                      <DayCell
                        day={d}
                        am={cellStatus(d.am, bookings)}
                        pm={cellStatus(d.pm, bookings)}
                        label={describe(d)}
                        interactive={isDayFree(d) ? !!onFreeDay : !!onBooking}
                        onActivate={() => activate(d)}
                      />
                    </td>
                  ) : (
                    <td key={i} className="p-0" aria-hidden="true" />
                  ),
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!compact && (
        <ul
          className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs"
          aria-label={t('map.legend', { ns: 'translation' })}
        >
          {CALENDAR_LEGEND.map((s) => (
            <li key={s} className="flex items-center gap-1.5">
              <StatusGlyph status={s} />
              {t(`map.status.${s}`, { ns: 'translation' })}
            </li>
          ))}
          <li className="flex items-center gap-1.5 text-muted-foreground">
            {t('calendar.halves')}
          </li>
        </ul>
      )}
    </div>
  );
}

function bookingName(b: SpotCalendarBooking, canSeeCustomers: boolean): string {
  const name = canSeeCustomers ? b.occupant_label?.trim() : '';
  return name ? `${b.code} ${name}` : b.code;
}

function DayCell({
  day,
  am,
  pm,
  label,
  interactive,
  onActivate,
}: {
  day: SpotCalendarDay;
  am: SpotStatus;
  pm: SpotStatus;
  label: string;
  interactive: boolean;
  onActivate: () => void;
}) {
  const halves = (
    <>
      <Half status={am} side="left" />
      <Half status={pm} side="right" />
    </>
  );
  const box = 'flex size-6 overflow-hidden rounded-sm ring-1 ring-foreground/15';
  if (!interactive) {
    return (
      <span role="img" aria-label={label} title={label} className={box} data-date={day.date}>
        {halves}
      </span>
    );
  }
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      data-date={day.date}
      onClick={onActivate}
      className={cn(
        box,
        'cursor-pointer outline-none hover:ring-2 hover:ring-ring focus-visible:ring-2 focus-visible:ring-ring',
      )}
    >
      {halves}
    </button>
  );
}

/** Half a day: status colour plus its glyph (no glyph for free). */
function Half({ status, side }: { status: SpotStatus; side: 'left' | 'right' }) {
  return (
    <span
      aria-hidden="true"
      data-half={side}
      data-cal-status={status}
      className={cn(
        'flex h-full w-1/2 items-center justify-center',
        STATUS_CLASSES[status].bg,
        status === 'free' && 'opacity-60',
      )}
    >
      {status !== 'free' && <StatusGlyph status={status} className="size-3 ring-0" />}
    </span>
  );
}
