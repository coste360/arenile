import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@arenile/shared/ui/alert-dialog';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import { Textarea } from '@arenile/shared/ui/textarea';
import { FieldRow, FormActions, FormDialog, OptionSelect } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { TriangleAlertIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { describeProblem, type ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { useAnonymizeCustomer, useCreateCustomer, useUpdateCustomer, type Customer } from './api';
import { DuplicateHint } from './CustomerPicker';
import {
  anonymizeSchema,
  customerSchema,
  customerValues,
  LOCALES,
  toCustomerInput,
  type CustomerValues,
} from './customer.schema';

export interface CustomerFormDialogProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Customer to edit (If-Match on its version); absent = create. */
  customer?: Customer;
  onSaved?: (customer: Customer) => void;
  onReload?: () => void;
}

/** Create or edit a customer (spec 23: minimal data, separate optional marketing consent). */
export function CustomerFormDialog({
  open,
  onOpenChange,
  customer,
  ...rest
}: CustomerFormDialogProps) {
  const { t } = useTranslation('customers');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={customer ? t('form.editTitle') : t('form.newTitle')}
      description={t('form.description')}
      wide
    >
      {open && <CustomerForm customer={customer} onDone={() => onOpenChange(false)} {...rest} />}
    </FormDialog>
  );
}

function CustomerForm({
  establishmentId,
  customer,
  onSaved,
  onReload,
  onDone,
}: Omit<CustomerFormDialogProps, 'open' | 'onOpenChange'> & { onDone: () => void }) {
  const { t } = useTranslation('customers');
  const create = useCreateCustomer(establishmentId);
  const update = useUpdateCustomer(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<CustomerValues>({
    resolver: zodResolver(customerSchema),
    defaultValues: customerValues(customer),
  });
  const [lastName, email, phone] = useWatch({
    control: form.control,
    name: ['last_name', 'email', 'phone'],
  });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      const input = toCustomerInput(v, customer);
      const saved = customer
        ? await update.mutateAsync({ id: customer.id, version: customer.version, input })
        : await create.mutateAsync(input);
      toast.success(customer ? t('form.updated') : t('form.created', { name: saved.display_name }));
      onSaved?.(saved);
      onDone();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} onReload={onReload} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="last_name" label={t('fields.lastName')}>
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
          <FieldRow control={form.control} name="first_name" label={t('fields.firstName')} optional>
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
          <FieldRow control={form.control} name="phone" label={t('fields.phone')} optional>
            {(field) => (
              <Input type="tel" autoComplete="off" placeholder="+39 333 1234567" {...field} />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="email" label={t('fields.email')} optional>
            {(field) => <Input type="email" autoComplete="off" {...field} />}
          </FieldRow>
          <FieldRow control={form.control} name="locale" label={t('fields.locale')}>
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                options={LOCALES.map((l) => ({
                  value: l,
                  label: t(`common.languages.${l}`, { ns: 'translation' }),
                }))}
              />
            )}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="tags"
            label={t('fields.tags')}
            description={t('fields.tagsHelp')}
            optional
          >
            {(field) => <Input {...field} />}
          </FieldRow>
        </div>
        {!customer && (
          <DuplicateHint
            establishmentId={establishmentId}
            candidate={{ last_name: lastName, email, phone }}
            onUse={(c) => {
              onSaved?.(c);
              onDone();
            }}
          />
        )}
        <FieldRow
          control={form.control}
          name="notes"
          label={t('fields.notes')}
          description={t('fields.notesHelp')}
          optional
        >
          {(field) => <Textarea rows={3} {...field} />}
        </FieldRow>
        <FieldRow
          control={form.control}
          name="marketing"
          label={t('fields.marketing')}
          description={
            customer?.marketing_consent
              ? t('consent.given', {
                  at: new Date(customer.marketing_consent.given_at).toLocaleDateString('it-IT'),
                  source: customer.marketing_consent.source,
                })
              : t('fields.marketingHelp')
          }
        >
          {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
        </FieldRow>
        <FormActions onCancel={onDone} pending={create.isPending || update.isPending} />
      </form>
    </Form>
  );
}

export interface AnonymizeDialogProps {
  establishmentId: string;
  customer: Customer;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onReload?: () => void;
}

/**
 * GDPR anonymisation (spec 23) behind a strong confirmation: the action is
 * irreversible, so the operator must type the customer's surname before the
 * button is enabled. Bookings and payments stay for accounting.
 */
export function AnonymizeDialog({
  establishmentId,
  customer,
  open,
  onOpenChange,
  onReload,
}: AnonymizeDialogProps) {
  const { t } = useTranslation('customers');
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t('anonymize.title', { name: customer.display_name })}
          </AlertDialogTitle>
          <AlertDialogDescription>{t('anonymize.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        {open && (
          <AnonymizeForm
            establishmentId={establishmentId}
            customer={customer}
            onDone={() => onOpenChange(false)}
            onReload={onReload}
          />
        )}
      </AlertDialogContent>
    </AlertDialog>
  );
}

function AnonymizeForm({
  establishmentId,
  customer,
  onDone,
  onReload,
}: {
  establishmentId: string;
  customer: Customer;
  onDone: () => void;
  onReload?: () => void;
}) {
  const { t } = useTranslation('customers');
  const anonymize = useAnonymizeCustomer(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const schema = anonymizeSchema(customer.last_name);
  const form = useForm<{ confirmation: string }>({
    resolver: zodResolver(schema),
    defaultValues: { confirmation: '' },
  });
  const typed = useWatch({ control: form.control, name: 'confirmation' });
  const matches = schema.safeParse({ confirmation: typed }).success;
  const onSubmit = form.handleSubmit(async () => {
    setFailure(null);
    try {
      await anonymize.mutateAsync({ id: customer.id, version: customer.version });
      toast.success(t('anonymize.done'));
      onDone();
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('anonymize.irreversible')}</AlertTitle>
          <AlertDescription>{t('anonymize.keeps')}</AlertDescription>
        </Alert>
        <ProblemAlert problem={failure} onReload={onReload} />
        <FieldRow
          control={form.control}
          name="confirmation"
          label={t('anonymize.typeLabel', { lastName: customer.last_name })}
        >
          {(field) => <Input autoComplete="off" {...field} />}
        </FieldRow>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={anonymize.isPending}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </AlertDialogCancel>
          <Button type="submit" variant="destructive" disabled={!matches || anonymize.isPending}>
            {anonymize.isPending && <Spinner />}
            {t('anonymize.confirm')}
          </Button>
        </AlertDialogFooter>
      </form>
    </Form>
  );
}
