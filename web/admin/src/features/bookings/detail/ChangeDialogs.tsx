import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { DatePicker, DateRangePicker, FormDialog, OptionSelect } from '@arenile/shared/ui/patterns';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useAvailability,
  useMoveBooking,
  usePatchBooking,
  type BookingChangeResult,
  type BookingDetail,
} from '../api';
import { changeDatesSchema } from '../booking.schema';
import { defaultFormula, SLOTS, todayIso, type Slots } from '../model';
import { describeProblem, formatConflicts, type ProblemText } from '../problem';
import { ProblemAlert } from '../ProblemAlert';
import { compareSpots, type EstablishmentSpots } from '../spots';
import { usePackName } from '../usePackName';

/** Previous total → new total and the difference to collect (+) or refund (−). */
export function ChangeSummary({
  result,
  inventory,
}: {
  result: BookingChangeResult;
  inventory: EstablishmentSpots;
}) {
  const { t } = useTranslation('bookings');
  const diff = result.price_difference;
  const conflicts = formatConflicts(result.conflicts, t, (g) =>
    g.kind === 'spot' ? inventory.spotById.get(g.resourceId)?.label : undefined,
  );
  return (
    <section
      aria-label={t('change.preview')}
      aria-live="polite"
      className="space-y-2 rounded-lg border p-3 text-sm"
    >
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1">
        <dt className="text-muted-foreground">{t('change.previous')}</dt>
        <dd className="text-right tabular-nums">{formatCents(result.previous_total)}</dd>
        <dt className="text-muted-foreground">{t('change.next')}</dt>
        <dd className="text-right tabular-nums">{formatCents(result.new_total)}</dd>
        <dt className="font-medium">{diff >= 0 ? t('change.toCollect') : t('change.toRefund')}</dt>
        <dd
          className={
            diff > 0
              ? 'text-right font-medium text-destructive tabular-nums'
              : 'text-right font-medium tabular-nums'
          }
          data-testid="change-difference"
        >
          {formatCents(Math.abs(diff))}
        </dd>
      </dl>
      {conflicts.length > 0 && (
        <div className="text-destructive">
          <p className="font-medium">{t('change.conflicts')}</p>
          <ul className="list-inside list-disc">
            {conflicts.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

interface ChangeDialogProps {
  establishmentId: string;
  booking: BookingDetail;
  inventory: EstablishmentSpots;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReload: () => void;
}

/** Spec 10.3: new dates/slot, re-quote preview (dry run), then atomic re-allocation. */
export function ChangeDatesDialog({
  establishmentId,
  booking,
  inventory,
  open,
  onOpenChange,
  onReload,
}: ChangeDialogProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('change.datesTitle')}
      description={t('change.datesDescription')}
    >
      {open && (
        <ChangeDatesForm
          establishmentId={establishmentId}
          booking={booking}
          inventory={inventory}
          onDone={() => onOpenChange(false)}
          onReload={onReload}
        />
      )}
    </FormDialog>
  );
}

function ChangeDatesForm({
  establishmentId,
  booking,
  inventory,
  onDone,
  onReload,
}: Omit<ChangeDialogProps, 'open' | 'onOpenChange'> & { onDone: () => void }) {
  const { t } = useTranslation('bookings');
  const patch = usePatchBooking(establishmentId);
  const [range, setRange] = useState({ from: booking.starts_on, to: booking.ends_on });
  const [slots, setSlots] = useState<Slots>(booking.slots);
  const [force, setForce] = useState(false);
  const [preview, setPreview] = useState<BookingChangeResult | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const parsed = changeDatesSchema.safeParse({
    starts_on: range.from,
    ends_on: range.to,
    slots,
    force,
  });
  const unchanged =
    range.from === booking.starts_on && range.to === booking.ends_on && slots === booking.slots;

  const body = () => {
    const auto =
      booking.formula === 'day' ||
      booking.formula === 'half_day' ||
      booking.formula === 'multi_day';
    return {
      starts_on: range.from,
      ends_on: range.to,
      slots,
      ...(auto ? { formula: defaultFormula(range.from, range.to, slots) } : {}),
      ...(force ? { force: true } : {}),
    };
  };
  const run = async (dryRun: boolean) => {
    setFailure(null);
    try {
      const result = await patch.mutateAsync({
        bookingId: booking.id,
        version: booking.version,
        patch: body(),
        dryRun,
      });
      if (dryRun) {
        setPreview(result);
      } else {
        toast.success(t('change.applied'));
        onDone();
      }
    } catch (err) {
      setFailure(
        describeProblem(err, t, {
          resourceLabel: (g) => inventory.spotById.get(g.resourceId)?.label,
        }),
      );
    }
  };
  const edit = (fn: () => void) => {
    fn();
    setPreview(null);
  };

  return (
    <div className="space-y-4">
      <ProblemAlert problem={failure} onReload={onReload} />
      <div className="space-y-1.5">
        <Label htmlFor="change-range">{t('fields.period')}</Label>
        <DateRangePicker
          id="change-range"
          value={range}
          onChange={(r) => r && edit(() => setRange({ from: r.from, to: r.to ?? r.from }))}
        />
      </div>
      <div className="space-y-1.5">
        <Label id="change-slots">{t('fields.slots')}</Label>
        <ToggleGroup
          type="single"
          variant="outline"
          aria-labelledby="change-slots"
          value={slots}
          onValueChange={(v) => v && edit(() => setSlots(v as Slots))}
        >
          {SLOTS.map((s) => (
            <ToggleGroupItem key={s} value={s}>
              {t(`slots.${s}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
      <div className="flex items-center gap-2">
        <Checkbox
          id="change-force"
          checked={force}
          onCheckedChange={(c) => edit(() => setForce(c === true))}
        />
        <Label htmlFor="change-force" className="font-normal">
          {t('change.force')}
        </Label>
      </div>
      {preview && <ChangeSummary result={preview} inventory={inventory} />}
      <DialogFooter className="border-t pt-4">
        <Button variant="outline" onClick={onDone}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        {preview && preview.conflicts.length === 0 ? (
          <Button onClick={() => void run(false)} disabled={patch.isPending}>
            {patch.isPending && <Spinner />}
            {t('change.apply')}
          </Button>
        ) : (
          <Button
            onClick={() => void run(true)}
            disabled={patch.isPending || !parsed.success || unchanged}
          >
            {patch.isPending && <Spinner />}
            {t('change.requote')}
          </Button>
        )}
      </DialogFooter>
    </div>
  );
}

export interface MoveDialogProps extends ChangeDialogProps {
  /** Line to move (preselected from the lines table). */
  lineId?: string;
}

/** Spec 11.3: move a line to another free spot (or assign a pending one), with a dry-run preview. */
export function MoveBookingDialog({ lineId, ...props }: MoveDialogProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('move.title')}
      description={t('move.description')}
    >
      {props.open && (
        <MoveForm {...props} initialLineId={lineId} onDone={() => props.onOpenChange(false)} />
      )}
    </FormDialog>
  );
}

function MoveForm({
  establishmentId,
  booking,
  inventory,
  initialLineId,
  onDone,
  onReload,
}: Omit<ChangeDialogProps, 'open' | 'onOpenChange'> & {
  initialLineId?: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('bookings');
  const move = useMoveBooking(establishmentId);
  const packName = usePackName(establishmentId);
  const spotLines = booking.lines.filter((l) => l.assignment_status !== 'not_applicable');
  const [lineId, setLineId] = useState(initialLineId ?? spotLines[0]?.id ?? '');
  const today = todayIso();
  const inStay = booking.starts_on < today && booking.ends_on >= today;
  const [from, setFrom] = useState<string | undefined>(inStay ? today : undefined);
  const [spotId, setSpotId] = useState('');
  const [preview, setPreview] = useState<BookingChangeResult | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const line = booking.lines.find((l) => l.id === lineId);
  const rangeFrom = from && from > booking.starts_on ? from : booking.starts_on;
  const availability = useAvailability(establishmentId, {
    from: rangeFrom,
    to: booking.ends_on,
    slots: booking.slots,
    pack_id: line?.pack_id,
  });
  const freeSpots = new Set(
    (availability.data?.zones ?? []).flatMap((z) =>
      (z.spots ?? []).filter((s) => s.status === 'available').map((s) => s.spot_id),
    ),
  );
  const options = inventory.spots
    .filter((s) => freeSpots.has(s.id) && s.id !== line?.spot_id)
    .sort(compareSpots)
    .map((s) => ({ value: s.id, label: `${s.label} · ${inventory.zoneName(s.zone_id)}` }));

  const run = async (dryRun: boolean) => {
    setFailure(null);
    try {
      const result = await move.mutateAsync({
        bookingId: booking.id,
        version: booking.version,
        request: {
          line_id: lineId,
          spot_id: spotId,
          ...(from ? { from } : {}),
          ...(dryRun ? { dry_run: true } : {}),
        },
      });
      if (dryRun) setPreview(result);
      else {
        toast.success(t('move.applied', { label: inventory.spotById.get(spotId)?.label ?? '' }));
        onDone();
      }
    } catch (err) {
      setFailure(
        describeProblem(err, t, {
          resourceLabel: (g) => inventory.spotById.get(g.resourceId)?.label,
        }),
      );
    }
  };

  return (
    <div className="space-y-4">
      <ProblemAlert problem={failure} onReload={onReload} />
      {spotLines.length > 1 && (
        <div className="space-y-1.5">
          <Label htmlFor="move-line">{t('move.line')}</Label>
          <OptionSelect
            id="move-line"
            value={lineId}
            onChange={(v) => {
              setLineId(v);
              setSpotId('');
              setPreview(null);
            }}
            options={spotLines.map((l) => ({
              value: l.id,
              label: `${l.spot_label ?? t('lines.pending')} · ${packName(l.pack_code)}`,
            }))}
          />
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor="move-from">{t('move.from')}</Label>
        <DatePicker
          id="move-from"
          value={from}
          min={booking.starts_on}
          max={booking.ends_on}
          clearable
          onChange={(v) => {
            setFrom(v);
            setPreview(null);
          }}
        />
        <p className="text-xs text-muted-foreground">{t('move.fromHint')}</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="move-spot">{t('move.spot')}</Label>
        <OptionSelect
          id="move-spot"
          value={spotId}
          onChange={(v) => {
            setSpotId(v);
            setPreview(null);
          }}
          options={options}
          placeholder={
            availability.isPending
              ? t('move.loading')
              : options.length
                ? t('move.pick')
                : t('move.noneFree')
          }
          disabled={options.length === 0}
        />
      </div>
      {preview && <ChangeSummary result={preview} inventory={inventory} />}
      <DialogFooter className="border-t pt-4">
        <Button variant="outline" onClick={onDone}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        {preview && preview.conflicts.length === 0 ? (
          <Button onClick={() => void run(false)} disabled={move.isPending}>
            {move.isPending && <Spinner />}
            {t('move.apply')}
          </Button>
        ) : (
          <Button onClick={() => void run(true)} disabled={move.isPending || !spotId || !lineId}>
            {move.isPending && <Spinner />}
            {t('move.check')}
          </Button>
        )}
      </DialogFooter>
    </div>
  );
}
