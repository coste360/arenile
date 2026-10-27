import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Switch } from '@arenile/shared/ui/switch';
import { Textarea } from '@arenile/shared/ui/textarea';
import {
  FieldRow,
  FormSection,
  FormSheet,
  OptionSelect,
  SheetForm,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { ACCOMMODATION_TYPES, PARTNER_KINDS, partners, type Partner } from './api';
import { describeM8Problem } from './lib';
import { partnerSchema, partnerValues, toPartnerInput, type PartnerValues } from './partner.schema';

export interface PartnerFormSheetProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Partner to edit (If-Match on its version); absent = create. */
  partner?: Partner;
  onSaved?: (partner: Partner) => void;
}

/** Create or edit a partner (spec 30.2.1): contacts optional, never logged. */
export function PartnerFormSheet({ open, onOpenChange, partner, ...rest }: PartnerFormSheetProps) {
  const { t } = useTranslation('partners');
  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      title={partner ? t('form.editTitle', { name: partner.name }) : t('form.newTitle')}
      description={t('form.description')}
      size="lg"
      padded={false}
    >
      {open && <PartnerForm partner={partner} onDone={() => onOpenChange(false)} {...rest} />}
    </FormSheet>
  );
}

function PartnerForm({
  establishmentId,
  partner,
  onSaved,
  onDone,
}: Omit<PartnerFormSheetProps, 'open' | 'onOpenChange'> & { onDone: () => void }) {
  const { t } = useTranslation('partners');
  const create = partners.useCreate(establishmentId);
  const update = partners.useUpdate(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<PartnerValues>({
    resolver: zodResolver(partnerSchema),
    defaultValues: partnerValues(partner),
  });
  const kind = useWatch({ control: form.control, name: 'kind' });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      const input = toPartnerInput(v);
      const saved = partner
        ? await update.mutateAsync({ id: partner.id, version: partner.version, input })
        : await create.mutateAsync(input);
      toast.success(partner ? t('form.updated') : t('form.created', { name: saved.name }));
      onSaved?.(saved);
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  });
  const text = (name: keyof PartnerValues, label: string, type = 'text') => (
    <FieldRow control={form.control} name={name} label={label} optional>
      {(field) => (
        <Input type={type} autoComplete="off" {...field} value={(field.value as string) ?? ''} />
      )}
    </FieldRow>
  );
  return (
    <Form {...form}>
      <SheetForm
        onSubmit={(e) => void onSubmit(e)}
        onCancel={onDone}
        pending={create.isPending || update.isPending}
      >
        <ProblemAlert problem={failure} />
        <FormSection title={t('form.general')}>
          <FieldRow control={form.control} name="name" label={t('fields.name')}>
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
          <div className="grid gap-4 sm:grid-cols-2">
            <FieldRow control={form.control} name="kind" label={t('fields.kind')}>
              {(field) => (
                <OptionSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={PARTNER_KINDS.map((k) => ({ value: k, label: t(`kinds.${k}`) }))}
                />
              )}
            </FieldRow>
            {kind === 'struttura_ricettiva' && (
              <FieldRow
                control={form.control}
                name="accommodation_type"
                label={t('fields.accommodationType')}
                optional
              >
                {(field) => (
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    placeholder={t('fields.accommodationPlaceholder')}
                    options={ACCOMMODATION_TYPES.map((k) => ({
                      value: k,
                      label: t(`accommodation.${k}`),
                    }))}
                  />
                )}
              </FieldRow>
            )}
            {text('municipality', t('fields.municipality'))}
            {text('address', t('fields.address'))}
          </div>
        </FormSection>
        <FormSection title={t('form.contacts')} description={t('form.contactsHelp')}>
          <div className="grid gap-4 sm:grid-cols-2">
            {text('contact_name', t('fields.contactName'))}
            {text('phone', t('fields.phone'), 'tel')}
            {text('email', t('fields.email'), 'email')}
          </div>
        </FormSection>
        <FormSection title={t('form.billing')} description={t('form.billingHelp')}>
          <div className="grid gap-4 sm:grid-cols-2">
            {text('vat_number', t('fields.vatNumber'))}
            {text('tax_code', t('fields.taxCode'))}
          </div>
          <FieldRow control={form.control} name="notes" label={t('fields.notes')} optional>
            {(field) => <Textarea rows={3} {...field} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="active"
            label={t('fields.active')}
            description={t('fields.activeHelp')}
          >
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
        </FormSection>
      </SheetForm>
    </Form>
  );
}
