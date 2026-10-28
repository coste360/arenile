import { Button } from '@arenile/shared/ui/button';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { CalendarRangeIcon, HandshakeIcon, UsersRoundIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { useTranslation } from 'react-i18next';
import { ReleaseRangeForm } from '../../passes/ReleaseRangeForm';
import { todayIn } from '../lib/dates';
import type { OperationsPermissions } from '../lib/permissions';
import { slotStatus, type OperationalSpot, type SlotChoice } from '../lib/status';
import { useCurrentEstablishment } from '../../../establishment';
import { SpotCalendarButton, type NewStayFromCalendar } from './SpotCalendarDialog';

export interface SpotSheetExtrasProps {
  establishmentId: string;
  spot: OperationalSpot;
  slot: SlotChoice;
  permissions: OperationsPermissions;
  /** Spots of the same group on the day (group move needs at least two). */
  groupSize: number;
  onNewBooking: (stay: NewStayFromCalendar) => void;
  onGroupMove: (spot: OperationalSpot) => void;
  onClose: () => void;
}

/**
 * M8 additions to the quick sheet of a spot (spec 30.4, 30.5, 30.7, 30.8):
 * agreement, partner and group of the booking, the season calendar of the
 * spot, the release of a range of a season pass and the group move.
 */
export function SpotSheetExtras({
  establishmentId,
  spot,
  slot,
  permissions,
  groupSize,
  onNewBooking,
  onGroupMove,
  onClose,
}: SpotSheetExtrasProps) {
  const { t } = useTranslation('operations');
  const { establishment } = useCurrentEstablishment();
  const [release, setRelease] = useState(false);
  const b = spot.booking;
  const status = slotStatus(spot, slot);
  const canRelease =
    !!b && b.is_season_pass && status === 'pass_holder' && permissions.canWriteBookings;
  const canGroupMove =
    !!b?.group_name && !!b.line_id && groupSize > 1 && permissions.canRun && !b.ticket;

  return (
    <div className="space-y-3" data-testid="spot-sheet-extras">
      {b && (b.agreement || b.partner || b.group_name || b.setup_label || b.party_size) && (
        <dl className="space-y-1.5 text-sm">
          {b.agreement && (
            <Fact label={t('sheet.agreement')}>
              <HandshakeIcon aria-hidden="true" className="mr-1 inline size-4" />
              {b.agreement.name}
            </Fact>
          )}
          {b.partner && !b.agreement && <Fact label={t('sheet.partner')}>{b.partner.name}</Fact>}
          {b.group_name && (
            <Fact label={t('sheet.group')}>
              <UsersRoundIcon aria-hidden="true" className="mr-1 inline size-4" />
              {b.group_name}
            </Fact>
          )}
          {!!b.party_size && <Fact label={t('sheet.party')}>{b.party_size}</Fact>}
          {b.setup_label && <Fact label={t('sheet.setup')}>{b.setup_label}</Fact>}
          {b.segment_starts_on && b.segment_ends_on && (
            <Fact label={t('sheet.segment')}>
              {t('sheet.segmentDates', {
                from: formatIsoDate(b.segment_starts_on),
                to: formatIsoDate(b.segment_ends_on),
              })}
            </Fact>
          )}
        </dl>
      )}
      <div className="flex flex-wrap gap-2">
        <SpotCalendarButton
          establishmentId={establishmentId}
          spot={spot}
          canSeeCustomers={permissions.canSeeCustomers}
          onNewBooking={
            permissions.canWriteBookings
              ? (stay) => {
                  onClose();
                  onNewBooking(stay);
                }
              : undefined
          }
        />
        {canRelease && (
          <Button variant="outline" onClick={() => setRelease(true)}>
            <CalendarRangeIcon aria-hidden="true" />
            {t('actions.releaseRange')}
          </Button>
        )}
        {canGroupMove && (
          <Button variant="outline" onClick={() => onGroupMove(spot)}>
            <UsersRoundIcon aria-hidden="true" />
            {t('actions.groupMove', { count: groupSize })}
          </Button>
        )}
      </div>
      {canRelease && b && (
        <FormDialog
          open={release}
          onOpenChange={setRelease}
          title={t('release.rangeTitle', { label: spot.label })}
          description={t('release.rangeDescription', { customer: b.customer_display_name })}
        >
          {release && (
            <ReleaseRangeForm
              establishmentId={establishmentId}
              pass={{
                id: b.booking_id,
                starts_on: b.starts_on,
                ends_on: b.ends_on,
                slots: b.slots,
              }}
              today={todayIn(establishment?.timezone)}
              onDone={() => {
                setRelease(false);
                onClose();
              }}
            />
          )}
        </FormDialog>
      )}
    </div>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right font-medium">{children}</dd>
    </div>
  );
}
