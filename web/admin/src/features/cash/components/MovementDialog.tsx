import { problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Input } from '@arenile/shared/ui/input';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { FormActions, FormDialog, MoneyInput } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { newIdempotencyKey, useAddMovement } from '../api';
import { movementSchema, type MovementValues } from '../schemas';
import { fieldAria } from '../lib/aria';
import { CashField } from './fields';

/** Cash in (versamento) or out (prelievo) with a mandatory reason. */
export function MovementDialog({
  establishmentId,
  sessionId,
  open,
  onOpenChange,
}: {
  establishmentId: string;
  sessionId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('cash');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('movement.title')}
      description={t('movement.description')}
    >
      {open && (
        <MovementForm
          establishmentId={establishmentId}
          sessionId={sessionId}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function MovementForm({
  establishmentId,
  sessionId,
  onDone,
}: {
  establishmentId: string;
  sessionId: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('cash');
  // One key per dialog: a retry after a network error is not recorded twice.
  const [idempotencyKey] = useState(newIdempotencyKey);
  const add = useAddMovement(establishmentId, sessionId);
  const form = useForm<MovementValues>({
    resolver: zodResolver(movementSchema),
    defaultValues: { kind: 'in', amount: null, reason: '' },
  });
  const errors = form.formState.errors;
  const submit = form.handleSubmit(async (v) => {
    try {
      await add.mutateAsync({
        idempotencyKey,
        kind: v.kind,
        amount: v.amount as number,
        reason: v.reason.trim(),
      });
      toast.success(t('movement.recorded'));
      onDone();
    } catch {
      // Rendered from add.error.
    }
  });
  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      {add.error && (
        <Alert variant="destructive">
          <AlertDescription>
            {t(problemMessageKey(add.error), { ns: 'translation' })}
          </AlertDescription>
        </Alert>
      )}
      <Controller
        control={form.control}
        name="kind"
        render={({ field }) => (
          <div className="space-y-2">
            <span id="mv-kind-label" className="text-sm font-medium">
              {t('movement.kind')}
            </span>
            <ToggleGroup
              type="single"
              variant="outline"
              aria-labelledby="mv-kind-label"
              value={field.value}
              onValueChange={(v) => v && field.onChange(v)}
            >
              <ToggleGroupItem value="in">{t('movement.in')}</ToggleGroupItem>
              <ToggleGroupItem value="out">{t('movement.out')}</ToggleGroupItem>
            </ToggleGroup>
          </div>
        )}
      />
      <CashField id="mv-amount" label={t('fields.amount')} error={errors.amount}>
        <Controller
          control={form.control}
          name="amount"
          render={({ field }) => (
            <MoneyInput
              {...fieldAria('mv-amount', errors.amount)}
              value={field.value}
              onChange={(v) => field.onChange(v)}
              onBlur={field.onBlur}
            />
          )}
        />
      </CashField>
      <CashField id="mv-reason" label={t('movement.reason')} error={errors.reason}>
        <Input
          {...fieldAria('mv-reason', errors.reason)}
          placeholder={t('movement.reasonPlaceholder')}
          {...form.register('reason')}
        />
      </CashField>
      <FormActions onCancel={onDone} pending={add.isPending} submitLabel={t('movement.submit')} />
    </form>
  );
}
