import { Checkbox } from '@arenile/shared/ui/checkbox';
import { Form } from '@arenile/shared/ui/form';
import { mapProblemToForm } from '@arenile/shared/api';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { Textarea } from '@arenile/shared/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  DateRangePicker,
  ExtraList,
  ExtraRow,
  FieldRow,
  FormSection,
  FormSheet,
  MoneyInput,
  OptionSelect,
  QuantityStepper,
  SheetForm,
  ResponsiveTabsList,
  useExtraUnitText,
  useQuantityMaxText,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from '@tanstack/react-router';
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { extras as extrasResource, packs as packsResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { newIdempotencyKey } from '../configuration/lib/validation';
import type { Customer } from '../customers/api';
import { CustomerPicker, DuplicateHint } from '../customers/CustomerPicker';
import { useDebounced } from '../customers/lib';
import { OnlineOnlyNotice } from '../../offline/OnlineOnlyNotice';
import { useBookingQuote, useCreateBooking, useRecordPayment } from './api';
import {
  makeNewBookingSchema,
  newBookingValues,
  toBookingCreate,
  toInitialPayment,
  toSelection,
  type ExtraLimits,
  type NewBookingDefaults,
  type NewBookingValues,
} from './booking.schema';
import { defaultFormula, FORMULAS, MANUAL_METHODS, SLOTS, todayIso, type Slots } from './model';
import { resolveBookingField } from './formErrors';
import { describeProblem, type ConflictGroup, type ProblemText } from './problem';
import { ProblemAlert } from './ProblemAlert';
import { QuotePreview } from './QuotePreview';
import { SpotPicker } from './SpotPicker';
import { useEstablishmentSpots } from './spots';

export interface NewBookingSheetProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Prefill from the map or the planning (selected spots and dates). */
  defaults?: NewBookingDefaults;
  /** Called with the new booking id once created (default: a toast linking to it). */
  onCreated?: (bookingId: string) => void;
}

/**
 * Side sheet to create a booking from the panel (spec 10, 15.1): dates and
 * slot, spots on the mini map or a zone/quantity, pack and extras, customer
 * (search or inline with a duplicate hint), live quote with policy, payment
 * now or later. Submits with an Idempotency-Key that stays the same while the
 * request body does not change, so a double click or a retry never creates
 * two bookings. Operations and planning open it with `defaults`.
 */
export function NewBookingSheet({
  establishmentId,
  open,
  onOpenChange,
  defaults,
  onCreated,
}: NewBookingSheetProps) {
  const { t } = useTranslation('bookings');
  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t('new.title')}
      description={t('new.description')}
      size="lg"
      padded={false}
    >
      {open && (
        <NewBookingForm
          establishmentId={establishmentId}
          defaults={defaults}
          onClose={() => onOpenChange(false)}
          onCreated={onCreated}
        />
      )}
    </FormSheet>
  );
}

function NewBookingForm({
  establishmentId,
  defaults,
  onClose,
  onCreated,
}: {
  establishmentId: string;
  defaults?: NewBookingDefaults;
  onClose: () => void;
  onCreated?: (bookingId: string) => void;
}) {
  const { t, i18n } = useTranslation('bookings');
  const navigate = useNavigate();
  const locale = i18n.language || 'it';
  const name = (text: Record<string, string>) => localize(text, locale, 'it');
  const inventory = useEstablishmentSpots(establishmentId, locale);
  const packList = packsResource.useList(establishmentId);
  const extraList = extrasResource.useList(establishmentId);
  const create = useCreateBooking(establishmentId);
  const pay = useRecordPayment(establishmentId);
  const [customer, setCustomer] = useState<Customer | null>(defaults?.customer ?? null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const keyRef = useRef<{ body: string; key: string } | null>(null);
  const unitText = useExtraUnitText();
  const maxText = useQuantityMaxText();

  // The schema mirrors the catalog limits (max_quantity) loaded after mount.
  const limits = useMemo<ExtraLimits>(
    () => Object.fromEntries((extraList.data ?? []).map((x) => [x.id, x.max_quantity])),
    [extraList.data],
  );
  const limitsRef = useRef(limits);
  useEffect(() => {
    limitsRef.current = limits;
  }, [limits]);
  const form = useForm<NewBookingValues>({
    resolver: (data, context, options) =>
      zodResolver(makeNewBookingSchema(limitsRef.current))(data, context, options),
    defaultValues: newBookingValues(todayIso(), defaults),
  });
  const values = useWatch({ control: form.control }) as NewBookingValues;

  const packs = (packList.data ?? [])
    .filter((p) => p.active)
    .sort((a, b) => a.sort_order - b.sort_order);
  const extras = (extraList.data ?? [])
    .filter(
      (x) =>
        x.active && (!x.allowed_pack_ids?.length || x.allowed_pack_ids.includes(values.pack_id)),
    )
    .sort((a, b) => a.sort_order - b.sort_order);
  const lineExtraIds = useMemo(
    () => new Set((extraList.data ?? []).filter((x) => x.attach_to === 'line').map((x) => x.id)),
    [extraList.data],
  );
  const zoneOptions = inventory.zones
    .filter((z) => z.assignment_mode !== 'seat')
    .map((z) => ({ value: z.id, label: name(z.name) || z.code }));
  const selectedZone = inventory.zoneById.get(values.zone_id);

  // A single pack is the obvious choice.
  useEffect(() => {
    if (!form.getValues('pack_id') && packs.length === 1 && packs[0]) {
      form.setValue('pack_id', packs[0].id);
    }
  }, [packs, form]);

  const lineCtx = {
    zoneOfSpot: (id: string) => inventory.spotById.get(id)?.zone_id,
    lineExtraIds,
    // Extras hidden by the chosen pack are never sent.
    extraIds: new Set(extras.map((x) => x.id)),
    holdId: defaults?.holdId,
  };
  const extraById = new Map((extraList.data ?? []).map((x) => [x.id, x]));
  const selection = toSelection(values, lineCtx);
  const quoteBody = useDebounced(selection ? JSON.stringify(selection) : null, 350);
  const quote = useBookingQuote(establishmentId, quoteBody ? JSON.parse(quoteBody) : null);
  const quoteStale =
    quote.isFetching || quoteBody !== (selection ? JSON.stringify(selection) : null);

  const resourceLabel = (g: ConflictGroup) =>
    g.kind === 'spot'
      ? inventory.spotById.get(g.resourceId)?.label
      : g.kind === 'zone'
        ? inventory.zoneName(g.resourceId)
        : undefined;

  const setDates = (from: string, to: string, slots: Slots) => {
    form.setValue('starts_on', from, { shouldValidate: true });
    form.setValue('ends_on', to, { shouldValidate: true });
    form.setValue('slots', slots);
    const current = form.getValues('formula');
    if (current === 'day' || current === 'half_day' || current === 'multi_day') {
      form.setValue('formula', defaultFormula(from, to, slots));
    }
  };

  const submitBooking = async (v: NewBookingValues) => {
    setFailure(null);
    const body = toBookingCreate(v, lineCtx);
    if (!body) return;
    const serialized = JSON.stringify(body);
    if (keyRef.current?.body !== serialized) {
      keyRef.current = { body: serialized, key: newIdempotencyKey() };
    }
    let bookingId: string;
    let code: string;
    try {
      const created = await create.mutateAsync({ body, idempotencyKey: keyRef.current.key });
      bookingId = created.id;
      code = created.code;
    } catch (err) {
      // Server field errors go on the fields (extras by id, not by position)
      // and the alert lists them in plain words, never as raw paths.
      const mapped = mapProblemToForm(err, form.setError, {
        resolve: (path, message) =>
          resolveBookingField(path, message, {
            body,
            inlineCustomer: v.customer_mode === 'new',
            extraName: (id) => {
              const x = extraById.get(id);
              return x ? name(x.name) || x.code : undefined;
            },
            extraMax: (id) => extraById.get(id)?.max_quantity,
            t,
          }),
      });
      setFailure(
        describeProblem(err, t, {
          resourceLabel,
          fieldErrors: mapped.errors.map((e) => e.text),
        }),
      );
      return;
    }
    keyRef.current = null;
    const payment = toInitialPayment(v);
    if (payment) {
      try {
        await pay.mutateAsync({ bookingId, input: payment });
      } catch (err) {
        // The booking exists: report the payment failure and let the operator retry from the detail.
        toast.warning(t('new.paymentFailed', { code }), {
          description: describeProblem(err, t).title,
        });
      }
    }
    onClose();
    if (onCreated) {
      onCreated(bookingId);
    } else {
      toast.success(t('new.created', { code }), {
        action: {
          label: t('new.open'),
          onClick: () =>
            void navigate({
              to: '/e/$establishmentId/prenotazioni/$bookingId',
              params: { establishmentId, bookingId },
            }),
        },
      });
    }
  };
  const onSubmit = (e?: FormEvent) => {
    // Never offline: only the server guarantees availability (spec 21.2).
    if (!navigator.onLine) return e?.preventDefault();
    return form.handleSubmit(submitBooking)(e);
  };

  const violations = quote.data?.policy.violations ?? [];
  const errors = form.formState.errors;

  return (
    <Form {...form}>
      <SheetForm
        onSubmit={(e) => void onSubmit(e)}
        onCancel={onClose}
        pending={create.isPending || pay.isPending}
        submitLabel={t('new.submit')}
      >
        <OnlineOnlyNotice />
        <ProblemAlert problem={failure} />

        <FormSection title={t('new.when')}>
          <FieldRow control={form.control} name="ends_on" label={t('fields.period')}>
            {() => (
              <DateRangePicker
                id="new-booking-period"
                value={{ from: values.starts_on, to: values.ends_on }}
                onChange={(r) => r && setDates(r.from, r.to ?? r.from, values.slots as Slots)}
              />
            )}
          </FieldRow>
          <div className="grid gap-4">
            <div className="space-y-2">
              <Label id="new-booking-slots">{t('fields.slots')}</Label>
              <ToggleGroup
                type="single"
                variant="outline"
                aria-labelledby="new-booking-slots"
                value={values.slots}
                onValueChange={(s) => s && setDates(values.starts_on, values.ends_on, s as Slots)}
              >
                {SLOTS.map((s) => (
                  <ToggleGroupItem key={s} value={s}>
                    {t(`slots.${s}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            <FieldRow control={form.control} name="formula" label={t('fields.formula')}>
              {(field) => (
                <OptionSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={FORMULAS.filter((f) => f !== 'season').map((f) => ({
                    value: f,
                    label: t(`formula.${f}`),
                  }))}
                />
              )}
            </FieldRow>
          </div>
        </FormSection>

        <FormSection title={t('new.where')}>
          <FieldRow control={form.control} name="pack_id" label={t('fields.pack')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                placeholder={t('fields.packPlaceholder')}
                options={packs.map((p) => ({ value: p.id, label: name(p.name) || p.code }))}
              />
            )}
          </FieldRow>
          <Tabs
            value={values.target}
            onValueChange={(v) => form.setValue('target', v as NewBookingValues['target'])}
          >
            <ResponsiveTabsList
              label={t('new.target')}
              value={values.target}
              onValueChange={(v) => form.setValue('target', v as NewBookingValues['target'])}
              options={[
                { value: 'spots', label: t('new.targetSpots') },
                { value: 'zone', label: t('new.targetZone'), disabled: zoneOptions.length === 0 },
              ]}
            />
            <TabsContent value="spots" className="pt-2">
              <SpotPicker
                establishmentId={establishmentId}
                inventory={inventory}
                from={values.starts_on}
                to={values.ends_on}
                slots={values.slots as Slots}
                packId={values.pack_id}
                value={values.spot_ids}
                onChange={(ids) =>
                  form.setValue('spot_ids', ids, { shouldValidate: !!errors.spot_ids })
                }
                allowIds={defaults?.holdId ? defaults.spotIds : undefined}
              />
              {errors.spot_ids?.message && (
                <p className="mt-2 text-sm text-destructive" role="alert">
                  {t(errors.spot_ids.message)}
                </p>
              )}
            </TabsContent>
            <TabsContent value="zone" className="grid gap-4 pt-2 sm:grid-cols-2">
              <FieldRow control={form.control} name="zone_id" label={t('fields.zone')}>
                {(field) => (
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    options={zoneOptions}
                  />
                )}
              </FieldRow>
              {selectedZone?.assignment_mode === 'quantity' && (
                <FieldRow control={form.control} name="quantity" label={t('fields.quantity')}>
                  {(field) => (
                    <QuantityStepper
                      label={t('fields.quantity')}
                      min={1}
                      max={100}
                      value={field.value}
                      onChange={field.onChange}
                    />
                  )}
                </FieldRow>
              )}
            </TabsContent>
          </Tabs>
          {extras.length > 0 && (
            <ExtraList title={t('fields.extras')}>
              {extras.map((x) => {
                const id = `extra-${x.id}`;
                const label = name(x.name) || x.code;
                const fieldError = errors.extras?.[x.id];
                // Server errors are plain text already; client ones are i18n keys.
                const error =
                  fieldError?.message &&
                  (fieldError.type === 'server'
                    ? fieldError.message
                    : t(fieldError.message, { max: x.max_quantity }));
                return (
                  <ExtraRow
                    key={x.id}
                    name={label}
                    detail={unitText(x)}
                    hint={maxText(x.max_quantity)}
                    error={error || undefined}
                    messageId={`${id}-message`}
                  >
                    <QuantityStepper
                      id={id}
                      label={label}
                      min={0}
                      max={x.max_quantity}
                      value={values.extras?.[x.id] ?? 0}
                      aria-invalid={!!error}
                      aria-describedby={`${id}-message`}
                      onChange={(q) => {
                        form.clearErrors(`extras.${x.id}`);
                        form.setValue('extras', { ...form.getValues('extras'), [x.id]: q });
                      }}
                    />
                  </ExtraRow>
                );
              })}
            </ExtraList>
          )}
        </FormSection>

        <FormSection title={t('new.customer')}>
          <ToggleGroup
            type="single"
            variant="outline"
            aria-label={t('new.customer')}
            value={values.customer_mode}
            onValueChange={(m) => {
              if (!m) return;
              form.setValue('customer_mode', m as NewBookingValues['customer_mode']);
              if (m === 'new' && !form.getValues('customer')) {
                form.setValue('customer', { first_name: '', last_name: '', email: '', phone: '' });
              }
            }}
          >
            <ToggleGroupItem value="existing">{t('new.customerExisting')}</ToggleGroupItem>
            <ToggleGroupItem value="new">{t('new.customerNew')}</ToggleGroupItem>
          </ToggleGroup>
          {values.customer_mode === 'existing' ? (
            <FieldRow control={form.control} name="customer_id" label={t('fields.customer')}>
              {(field) => (
                <CustomerPicker
                  establishmentId={establishmentId}
                  value={customer}
                  invalid={!!errors.customer_id}
                  onChange={(c) => {
                    setCustomer(c);
                    field.onChange(c?.id ?? '');
                  }}
                />
              )}
            </FieldRow>
          ) : (
            <div className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FieldRow
                  control={form.control}
                  name="customer.last_name"
                  label={t('fields.lastName')}
                >
                  {(field) => <Input autoComplete="off" {...field} value={field.value ?? ''} />}
                </FieldRow>
                <FieldRow
                  control={form.control}
                  name="customer.first_name"
                  label={t('fields.firstName')}
                  optional
                >
                  {(field) => <Input autoComplete="off" {...field} value={field.value ?? ''} />}
                </FieldRow>
                <FieldRow
                  control={form.control}
                  name="customer.phone"
                  label={t('fields.phone')}
                  optional
                >
                  {(field) => (
                    <Input
                      type="tel"
                      autoComplete="off"
                      placeholder="+39 333 1234567"
                      {...field}
                      value={field.value ?? ''}
                    />
                  )}
                </FieldRow>
                <FieldRow
                  control={form.control}
                  name="customer.email"
                  label={t('fields.email')}
                  optional
                >
                  {(field) => (
                    <Input type="email" autoComplete="off" {...field} value={field.value ?? ''} />
                  )}
                </FieldRow>
              </div>
              <DuplicateHint
                establishmentId={establishmentId}
                candidate={values.customer ?? {}}
                onUse={(c) => {
                  setCustomer(c);
                  form.setValue('customer_mode', 'existing');
                  form.setValue('customer_id', c.id, { shouldValidate: true });
                }}
              />
            </div>
          )}
        </FormSection>

        <FormSection title={t('quote.title')}>
          <QuotePreview
            quote={quote.data}
            loading={quote.isFetching}
            error={quote.error}
            stale={quoteStale && !!quote.data}
            resourceLabel={resourceLabel}
          />
          {violations.length > 0 && (
            <div className="flex items-center gap-2">
              <Checkbox
                id="new-booking-force"
                checked={values.force}
                onCheckedChange={(c) => form.setValue('force', c === true)}
              />
              <Label htmlFor="new-booking-force" className="font-normal">
                {t('new.force')}
              </Label>
            </div>
          )}
        </FormSection>

        <FormSection title={t('new.payment')}>
          <div className="flex items-center gap-3">
            <Switch
              id="new-booking-pay-now"
              checked={values.pay_now}
              onCheckedChange={(on) => {
                form.setValue('pay_now', on);
                if (on && form.getValues('payment_amount') === null && quote.data) {
                  const due =
                    quote.data.deposit_due > 0
                      ? quote.data.deposit_due
                      : quote.data.breakdown.total;
                  form.setValue('payment_amount', due);
                }
              }}
            />
            <Label htmlFor="new-booking-pay-now">{t('new.payNow')}</Label>
          </div>
          {values.pay_now ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <FieldRow control={form.control} name="payment_amount" label={t('fields.amount')}>
                {(field) => (
                  <MoneyInput
                    value={field.value}
                    onChange={(c) => field.onChange(c)}
                    onBlur={field.onBlur}
                  />
                )}
              </FieldRow>
              <FieldRow control={form.control} name="payment_method" label={t('fields.method')}>
                {(field) => (
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    options={MANUAL_METHODS.filter((m) => m !== 'credit').map((m) => ({
                      value: m,
                      label: t(`method.${m}`),
                    }))}
                  />
                )}
              </FieldRow>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t('new.payLater')}</p>
          )}
          <FieldRow
            control={form.control}
            name="notes_internal"
            label={t('fields.notesInternal')}
            optional
          >
            {(field) => <Textarea rows={2} {...field} />}
          </FieldRow>
        </FormSection>
      </SheetForm>
    </Form>
  );
}
