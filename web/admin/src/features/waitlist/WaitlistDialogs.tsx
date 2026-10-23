import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import { Textarea } from '@arenile/shared/ui/textarea';
import {
  DateRangePicker,
  FormActions,
  FormDialog,
  OptionSelect,
  SegmentedControl,
  type IsoDateRange,
} from '@arenile/shared/ui/patterns';
import { useId, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { Slots } from '../bookings/model';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import type { EstablishmentSpots } from '../bookings/spots';
import { packs as packsResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { newIdempotencyKey } from '../configuration/lib/validation';
import type { Customer } from '../customers/api';
import { CustomerPicker } from '../customers/CustomerPicker';
import { describeM5bProblem } from '../renewals/problems';
import {
  useCreateWaitlistEntry,
  useOfferWaitlistEntry,
  useUpdateWaitlistEntry,
  useUpdateWaitlistSettings,
  type WaitlistEntry,
  type WaitlistSettings,
} from './api';

const SLOTS: readonly Slots[] = ['FULL', 'AM', 'PM'];

function Field({
  id,
  label,
  optional,
  error,
  children,
}: {
  id: string;
  label: string;
  optional?: boolean;
  error?: string;
  children: ReactNode;
}) {
  const { t } = useTranslation('waitlist');
  return (
    <div className="min-w-0 space-y-2">
      <Label id={`${id}-label`} htmlFor={id}>
        {label}
        {optional && <span className="font-normal text-muted-foreground"> {t('optional')}</span>}
      </Label>
      {children}
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

export interface EntryDialogProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Entry to edit; a new entry when absent. */
  entry?: WaitlistEntry;
  inventory: EstablishmentSpots;
}

/** New waitlist entry from the desk (spec 10.5), or edit of the request of an entry. */
export function EntryDialog(props: EntryDialogProps) {
  const { t } = useTranslation('waitlist');
  return (
    <FormDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.entry ? t('entry.editTitle') : t('entry.newTitle')}
      description={props.entry ? props.entry.customer_display_name : t('entry.newDescription')}
      wide
    >
      {props.open && <EntryForm {...props} />}
    </FormDialog>
  );
}

function EntryForm({ establishmentId, onOpenChange, entry, inventory }: EntryDialogProps) {
  const { t, i18n } = useTranslation('waitlist');
  const id = useId();
  const packs = packsResource.useList(establishmentId);
  const create = useCreateWaitlistEntry(establishmentId);
  const update = useUpdateWaitlistEntry(establishmentId);
  const [key] = useState(newIdempotencyKey);
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [period, setPeriod] = useState<IsoDateRange | undefined>(
    entry ? { from: entry.starts_on, to: entry.ends_on } : undefined,
  );
  const [slots, setSlots] = useState<Slots>(entry?.slots ?? 'FULL');
  const [packId, setPackId] = useState(entry?.pack_id ?? '');
  const [zoneId, setZoneId] = useState(entry?.zone_id ?? '');
  const [quantity, setQuantity] = useState(String(entry?.quantity ?? 1));
  const [note, setNote] = useState(entry?.note ?? '');
  const [submitted, setSubmitted] = useState(false);
  const [failure, setFailure] = useState<ProblemText | null>(null);

  const qty = Number(quantity);
  const errors = {
    customer: !entry && !customer ? t('entry.required') : undefined,
    period: !period?.from || !period.to ? t('entry.required') : undefined,
    pack: !packId ? t('entry.required') : undefined,
    quantity:
      !Number.isInteger(qty) || qty < 1 || qty > 20 ? t('entry.quantityInvalid') : undefined,
  };
  const shown = (e: string | undefined) => (submitted ? e : undefined);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    setFailure(null);
    if (Object.values(errors).some(Boolean) || !period?.from || !period.to) return;
    const request = {
      starts_on: period.from,
      ends_on: period.to,
      slots,
      pack_id: packId,
      zone_id: zoneId || undefined,
      quantity: qty,
      note: note.trim() || undefined,
    };
    try {
      if (entry) {
        await update.mutateAsync({ entry, body: request });
        toast.success(t('entry.updated'));
      } else {
        await create.mutateAsync({
          body: { ...request, customer_id: (customer as Customer).id, channel: 'desk' },
          idempotencyKey: key,
        });
        toast.success(t('entry.created', { customer: customer?.display_name ?? '' }));
      }
      onOpenChange(false);
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };

  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <div className="grid gap-4 sm:grid-cols-2">
        {!entry && (
          <Field id={`${id}-customer`} label={t('fields.customer')} error={shown(errors.customer)}>
            <CustomerPicker
              id={`${id}-customer`}
              establishmentId={establishmentId}
              value={customer}
              onChange={setCustomer}
              invalid={!!shown(errors.customer)}
            />
          </Field>
        )}
        <Field id={`${id}-period`} label={t('fields.period')} error={shown(errors.period)}>
          <DateRangePicker
            aria-labelledby={`${id}-period-label`}
            value={period}
            onChange={setPeriod}
            aria-invalid={!!shown(errors.period)}
          />
        </Field>
        <Field id={`${id}-slots`} label={t('fields.slots')}>
          <SegmentedControl
            label={t('fields.slots')}
            value={slots}
            onValueChange={(v) => setSlots(v as Slots)}
            options={SLOTS.map((s) => ({ value: s, label: t(`bookings:slots.${s}`) }))}
            collapseOnNarrow
          />
        </Field>
        <Field id={`${id}-pack`} label={t('fields.pack')} error={shown(errors.pack)}>
          <OptionSelect
            id={`${id}-pack`}
            value={packId}
            onChange={setPackId}
            placeholder={t('fields.pickPack')}
            options={(packs.data ?? []).map((p) => ({
              value: p.id,
              label: localize(p.name, i18n.language, 'it') || p.code,
            }))}
            aria-invalid={!!shown(errors.pack)}
          />
        </Field>
        <Field id={`${id}-zone`} label={t('fields.zone')} optional>
          <OptionSelect
            id={`${id}-zone`}
            value={zoneId}
            onChange={setZoneId}
            options={[
              { value: '', label: t('fields.anyZone') },
              ...inventory.zones.map((z) => ({
                value: z.id,
                label: inventory.zoneName(z.id) || z.code,
              })),
            ]}
          />
        </Field>
        <Field id={`${id}-qty`} label={t('fields.quantity')} error={shown(errors.quantity)}>
          <Input
            id={`${id}-qty`}
            inputMode="numeric"
            value={quantity}
            onChange={(e) => setQuantity(e.target.value)}
            aria-invalid={!!shown(errors.quantity)}
          />
        </Field>
      </div>
      <Field id={`${id}-note`} label={t('fields.note')} optional>
        <Textarea
          id={`${id}-note`}
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <FormActions
        onCancel={() => onOpenChange(false)}
        pending={create.isPending || update.isPending}
        submitLabel={entry ? t('entry.save') : t('entry.create')}
      />
    </form>
  );
}

/** Manual offer: a spot (or the first compatible one) held for the entry for N minutes. */
export function OfferDialog({
  establishmentId,
  entry,
  settings,
  inventory,
  onClose,
}: {
  establishmentId: string;
  entry: WaitlistEntry | null;
  settings: WaitlistSettings | undefined;
  inventory: EstablishmentSpots;
  onClose: () => void;
}) {
  const { t } = useTranslation('waitlist');
  return (
    <FormDialog
      open={!!entry}
      onOpenChange={(o) => !o && onClose()}
      title={t('offer.title')}
      description={entry ? t('offer.description', { customer: entry.customer_display_name }) : ''}
    >
      {entry && (
        <OfferForm
          establishmentId={establishmentId}
          entry={entry}
          defaultMinutes={settings?.offer_hold_minutes ?? 120}
          inventory={inventory}
          onDone={onClose}
        />
      )}
    </FormDialog>
  );
}

function OfferForm({
  establishmentId,
  entry,
  defaultMinutes,
  inventory,
  onDone,
}: {
  establishmentId: string;
  entry: WaitlistEntry;
  defaultMinutes: number;
  inventory: EstablishmentSpots;
  onDone: () => void;
}) {
  const { t } = useTranslation('waitlist');
  const id = useId();
  const offer = useOfferWaitlistEntry(establishmentId);
  const [key] = useState(newIdempotencyKey);
  const [spotId, setSpotId] = useState(entry.spot_id ?? '');
  const [minutes, setMinutes] = useState(String(defaultMinutes));
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const mins = Number(minutes);
  const minutesValid = Number.isInteger(mins) && mins >= 5 && mins <= 10080;
  const spots = inventory.spots.filter(
    (s) => s.active && (!entry.zone_id || s.zone_id === entry.zone_id),
  );
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!minutesValid) return;
    setFailure(null);
    try {
      await offer.mutateAsync({
        entryId: entry.id,
        body: { spot_id: spotId || undefined, hold_minutes: mins },
        idempotencyKey: key,
      });
      toast.success(t('offer.done', { customer: entry.customer_display_name }));
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };
  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <Field id={`${id}-spot`} label={t('offer.spot')} optional>
        <OptionSelect
          id={`${id}-spot`}
          value={spotId}
          onChange={setSpotId}
          options={[
            { value: '', label: t('offer.anySpot') },
            ...spots.map((s) => ({ value: s.id, label: s.label })),
          ]}
        />
      </Field>
      <Field
        id={`${id}-minutes`}
        label={t('offer.minutes')}
        error={minutesValid ? undefined : t('offer.minutesInvalid')}
      >
        <Input
          id={`${id}-minutes`}
          inputMode="numeric"
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
          aria-invalid={!minutesValid}
        />
      </Field>
      <p className="text-sm text-muted-foreground">{t('offer.help')}</p>
      <FormActions onCancel={onDone} pending={offer.isPending} submitLabel={t('offer.submit')} />
    </form>
  );
}

/** Waitlist settings (establishment.configure): on/off, public join, automatic offers, hold time. */
export function SettingsDialog({
  establishmentId,
  settings,
  open,
  onOpenChange,
}: {
  establishmentId: string;
  settings: WaitlistSettings;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('waitlist');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('settings.title')}
      description={t('settings.description')}
    >
      {open && (
        <SettingsForm
          establishmentId={establishmentId}
          settings={settings}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function SettingsForm({
  establishmentId,
  settings,
  onDone,
}: {
  establishmentId: string;
  settings: WaitlistSettings;
  onDone: () => void;
}) {
  const { t } = useTranslation('waitlist');
  const id = useId();
  const save = useUpdateWaitlistSettings(establishmentId);
  const [enabled, setEnabled] = useState(settings.enabled);
  const publicJoin = settings.public_join;
  const [autoOffer, setAutoOffer] = useState(settings.auto_offer);
  const [minutes, setMinutes] = useState(String(settings.offer_hold_minutes));
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const mins = Number(minutes);
  const minutesValid = Number.isInteger(mins) && mins >= 5 && mins <= 10080;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!minutesValid) return;
    setFailure(null);
    try {
      await save.mutateAsync({
        version: settings.version,
        body: {
          enabled,
          public_join: publicJoin,
          auto_offer: autoOffer,
          offer_hold_minutes: mins,
        },
      });
      toast.success(t('settings.saved'));
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };
  // public_join is kept as stored but not offered: the public booking page
  // has no join form yet (the public API accepts joins; see the guide).
  const toggles = [
    { key: 'enabled', value: enabled, set: setEnabled },
    { key: 'auto_offer', value: autoOffer, set: setAutoOffer },
  ] as const;
  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      {toggles.map((tg) => (
        <div key={tg.key} className="flex items-start gap-3">
          <Switch
            id={`${id}-${tg.key}`}
            checked={tg.value}
            onCheckedChange={tg.set}
            disabled={tg.key !== 'enabled' && !enabled}
          />
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`${id}-${tg.key}`}>{t(`settings.${tg.key}.label`)}</Label>
            <p className="text-sm text-muted-foreground">{t(`settings.${tg.key}.help`)}</p>
          </div>
        </div>
      ))}
      <Field
        id={`${id}-minutes`}
        label={t('settings.minutes')}
        error={minutesValid ? undefined : t('offer.minutesInvalid')}
      >
        <Input
          id={`${id}-minutes`}
          inputMode="numeric"
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
          aria-invalid={!minutesValid}
        />
      </Field>
      <FormActions onCancel={onDone} pending={save.isPending} />
    </form>
  );
}
