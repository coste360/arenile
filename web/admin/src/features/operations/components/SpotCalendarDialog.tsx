import { Button } from '@arenile/shared/ui/button';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@arenile/shared/ui/hover-card';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { useNavigate } from '@tanstack/react-router';
import { CalendarDaysIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { proposedStay, type SpotCalendar } from '../lib/spotCalendar';
import { SpotCalendarView } from './SpotCalendarView';

export interface NewStayFromCalendar {
  spotId: string;
  startsOn: string;
  endsOn: string;
  slots: 'FULL' | 'AM' | 'PM';
}

export interface SpotCalendarButtonProps {
  establishmentId: string;
  spot: { spot_id: string; label: string };
  canSeeCustomers: boolean;
  /** New booking from a free day (absent without bookings.write). */
  onNewBooking?: (stay: NewStayFromCalendar) => void;
}

/**
 * "Calendario" of the quick sheet (spec 30.5): hovering it with a mouse
 * previews the season calendar of the spot (desktop), a click or Enter opens
 * it in a dialog where a free day starts a new booking with the following
 * free days proposed and an occupied day opens its booking.
 */
export function SpotCalendarButton({
  establishmentId,
  spot,
  canSeeCustomers,
  onNewBooking,
}: SpotCalendarButtonProps) {
  const { t } = useTranslation('operations');
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);

  const openBooking = (bookingId: string) => {
    setOpen(false);
    void navigate({
      to: '/e/$establishmentId/prenotazioni/$bookingId',
      params: { establishmentId, bookingId },
    });
  };
  const newFrom = onNewBooking
    ? (cal: SpotCalendar, date: string) => {
        const stay = proposedStay(cal.days, date);
        if (!stay) return;
        setOpen(false);
        onNewBooking({ spotId: spot.spot_id, ...stay });
      }
    : undefined;

  return (
    <>
      <HoverCard openDelay={400} closeDelay={100}>
        <HoverCardTrigger asChild>
          <Button variant="outline" onClick={() => setOpen(true)}>
            <CalendarDaysIcon aria-hidden="true" />
            {t('calendar.open')}
          </Button>
        </HoverCardTrigger>
        {/* Pointer preview only: keyboard and touch users open the dialog. */}
        <HoverCardContent side="left" className="hidden w-auto max-w-[min(90vw,52rem)] md:block">
          <p className="mb-2 text-sm font-medium">{t('calendar.title', { label: spot.label })}</p>
          <SpotCalendarView
            establishmentId={establishmentId}
            spotId={spot.spot_id}
            canSeeCustomers={canSeeCustomers}
            compact
          />
        </HoverCardContent>
      </HoverCard>
      <FormDialog
        open={open}
        onOpenChange={setOpen}
        title={t('calendar.title', { label: spot.label })}
        description={t('calendar.description')}
        wide
      >
        {open && (
          <SpotCalendarView
            establishmentId={establishmentId}
            spotId={spot.spot_id}
            canSeeCustomers={canSeeCustomers}
            onFreeDay={newFrom}
            onBooking={openBooking}
          />
        )}
      </FormDialog>
    </>
  );
}
