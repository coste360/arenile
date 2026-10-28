import { Checkbox } from '@arenile/shared/ui/checkbox';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Textarea } from '@arenile/shared/ui/textarea';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  DateRangePicker,
  MoneyInput,
  OptionSelect,
  PercentInput,
  QuantityStepper,
} from '@arenile/shared/ui/patterns';
import { useId, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { effectiveAgreementId, type BookingAgreementValue, type ShareType } from './model';
import type { AgreementOptions } from './useAgreementOptions';

const NONE = '__none';
const SHARES: readonly ShareType[] = ['none', 'full', 'percent', 'amount'];

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: ReactNode;
  hint?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

/**
 * Agreement ("Convenzione"), partner ("ospite di Hotel Miramare", share
 * charged to the partner, stay at the partner's) and group of a booking
 * (spec 30.2.3, 30.2.4, 30.3). Controlled; used by the new booking sheet and
 * the booking detail. Guest data is limited to names (spec 23).
 */
export function BookingAgreementFields({
  value,
  onChange,
  options,
  customer,
  verify,
}: {
  value: BookingAgreementValue;
  onChange: (next: BookingAgreementValue) => void;
  options: AgreementOptions;
  /** Customer of the booking: its default agreement is proposed (spec 30.2.3). */
  customer?: { agreement_id?: string } | null;
  /** Show the "verificato" checkbox (new booking); the detail verifies with its own action. */
  verify?: boolean;
}) {
  const { t } = useTranslation('partners');
  const id = useId();
  const set = (patch: Partial<BookingAgreementValue>) => onChange({ ...value, ...patch });
  const agreementId = effectiveAgreementId(value, customer);
  const fromCustomer = value.agreementId === undefined && !!customer?.agreement_id;
  const requirement = options.selected?.requirement;

  return (
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id={`${id}-agreement`}
          label={t('booking.agreement')}
          hint={fromCustomer ? t('booking.agreementFromCustomer') : undefined}
        >
          <OptionSelect
            id={`${id}-agreement`}
            value={agreementId || NONE}
            onChange={(v) => set({ agreementId: v === NONE ? '' : v, requirementVerified: false })}
            options={[
              { value: NONE, label: t('booking.noAgreement') },
              ...options.agreements.map((a) => ({
                value: a.id,
                label: a.code ? `${a.name} · ${a.code}` : a.name,
              })),
            ]}
          />
        </Field>
        <Field id={`${id}-partner`} label={t('booking.partner')} hint={t('booking.partnerHint')}>
          <OptionSelect
            id={`${id}-partner`}
            value={value.partnerId || NONE}
            onChange={(v) => set({ partnerId: v === NONE ? '' : v })}
            options={[
              { value: NONE, label: t('booking.noPartner') },
              ...options.partners.map((p) => ({ value: p.id, label: p.name })),
            ]}
          />
        </Field>
      </div>

      {verify && agreementId && requirement && (
        <div className="flex items-start gap-2 rounded-lg border p-3">
          <Checkbox
            id={`${id}-verified`}
            checked={value.requirementVerified}
            onCheckedChange={(c) => set({ requirementVerified: c === true })}
          />
          <div className="space-y-1">
            <Label htmlFor={`${id}-verified`}>{t('booking.verified')}</Label>
            <p className="text-xs text-muted-foreground">
              {t('booking.verifiedHint', { requirement })}
            </p>
          </div>
        </div>
      )}

      {value.partnerId && (
        <div className="space-y-4 rounded-lg border p-3">
          <div className="space-y-2">
            <Label id={`${id}-share`}>{t('booking.share')}</Label>
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              className="flex-wrap"
              aria-labelledby={`${id}-share`}
              value={value.shareType}
              onValueChange={(v) => v && set({ shareType: v as ShareType })}
            >
              {SHARES.map((s) => (
                <ToggleGroupItem key={s} value={s}>
                  {t(`booking.shares.${s}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <p className="text-xs text-muted-foreground">{t('booking.shareHint')}</p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            {value.shareType === 'percent' && (
              <Field id={`${id}-percent`} label={t('booking.sharePercent')}>
                <PercentInput
                  id={`${id}-percent`}
                  value={value.sharePercentBp}
                  onChange={(bp) => set({ sharePercentBp: bp })}
                />
              </Field>
            )}
            {value.shareType === 'amount' && (
              <Field id={`${id}-amount`} label={t('booking.shareAmount')}>
                <MoneyInput
                  id={`${id}-amount`}
                  value={value.shareAmount}
                  onChange={(c) => set({ shareAmount: c })}
                />
              </Field>
            )}
            <Field id={`${id}-stay`} label={t('booking.guestStay')} hint={t('booking.optional')}>
              <DateRangePicker
                id={`${id}-stay`}
                clearable
                value={value.guestStay ?? null}
                onChange={(r) => set({ guestStay: r ? { from: r.from, to: r.to } : undefined })}
              />
            </Field>
          </div>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field id={`${id}-group`} label={t('booking.groupName')} hint={t('booking.optional')}>
          <Input
            id={`${id}-group`}
            autoComplete="off"
            maxLength={120}
            value={value.groupName}
            onChange={(e) => set({ groupName: e.target.value })}
          />
        </Field>
        {!value.partnerId && (
          <Field
            id={`${id}-origin`}
            label={t('booking.groupOrigin')}
            hint={t('booking.groupOriginHint')}
          >
            <Input
              id={`${id}-origin`}
              autoComplete="off"
              maxLength={120}
              value={value.groupOrigin}
              onChange={(e) => set({ groupOrigin: e.target.value })}
            />
          </Field>
        )}
        <Field id={`${id}-adults`} label={t('booking.adults')}>
          <QuantityStepper
            id={`${id}-adults`}
            label={t('booking.adults')}
            min={0}
            max={99}
            value={value.adults}
            onChange={(n) => set({ adults: n })}
          />
        </Field>
        <Field id={`${id}-children`} label={t('booking.children')}>
          <QuantityStepper
            id={`${id}-children`}
            label={t('booking.children')}
            min={0}
            max={99}
            value={value.children}
            onChange={(n) => set({ children: n })}
          />
        </Field>
      </div>
      <Field id={`${id}-names`} label={t('booking.guestNames')} hint={t('booking.guestNamesHint')}>
        <Textarea
          id={`${id}-names`}
          rows={2}
          value={value.guestNames}
          onChange={(e) => set({ guestNames: e.target.value })}
        />
      </Field>
    </div>
  );
}
