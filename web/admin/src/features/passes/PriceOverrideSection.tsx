import { useMe, useProblemText } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import {
  ConfirmDialog,
  FieldRow,
  FormActions,
  FormDialog,
  MoneyInput,
  SectionCard,
} from '@arenile/shared/ui/patterns';
import { Textarea } from '@arenile/shared/ui/textarea';
import { zodResolver } from '@hookform/resolvers/zod';
import { PencilIcon, ReceiptIcon, RotateCcwIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment, useInstantFormat } from '../../establishment';
import { permissionsFor } from '../configuration/lib/permissions';
import { usePassPriceOverride, type SeasonPass } from './api';
import { overrideSchema, type OverrideValues } from './override.schema';

/**
 * Price of a season pass with the manual override (ADR 0019 E): shown to
 * everybody, changed only by owner and manager (establishment.configure; the
 * server checks it again and writes the audit with before/after and reason).
 */
export function PriceOverrideSection({
  establishmentId,
  pass,
}: {
  establishmentId: string;
  pass: SeasonPass;
}) {
  const { t } = useTranslation('passes');
  const problemText = useProblemText();
  const fmt = useInstantFormat();
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  const canConfigure = permissionsFor(me.data, establishment).canConfigure;
  const editable = canConfigure && pass.status !== 'cancelled' && pass.status !== 'expired';
  const mutation = usePassPriceOverride(establishmentId);
  const [editing, setEditing] = useState(false);
  const [clearing, setClearing] = useState(false);
  const o = pass.price_override;

  const clear = async () => {
    await mutation.mutateAsync({ passId: pass.id, version: pass.version, body: null });
    toast.success(t('override.cleared'));
  };

  return (
    <SectionCard
      icon={ReceiptIcon}
      title={t('override.title')}
      description={o ? t('override.descriptionManual') : t('override.descriptionEngine')}
      actions={o ? <Badge variant="outline">{t('override.manual')}</Badge> : undefined}
      footer={
        editable ? (
          <>
            {o && (
              <Button variant="outline" onClick={() => setClearing(true)}>
                <RotateCcwIcon aria-hidden="true" />
                {t('override.clear')}
              </Button>
            )}
            <Button variant="outline" onClick={() => setEditing(true)}>
              <PencilIcon aria-hidden="true" />
              {o ? t('override.edit') : t('override.set')}
            </Button>
          </>
        ) : undefined
      }
    >
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <div>
          <dt className="text-muted-foreground">{t('override.current')}</dt>
          <dd className="font-medium tabular-nums">{formatCents(pass.total_amount)}</dd>
        </div>
        {o && (
          <>
            <div>
              <dt className="text-muted-foreground">{t('override.engine')}</dt>
              <dd className="tabular-nums">{formatCents(o.engine_amount)}</dd>
            </div>
            <div className="col-span-2">
              <dt className="text-muted-foreground">{t('override.reason')}</dt>
              <dd className="break-words">{o.reason}</dd>
            </div>
            <div className="col-span-2">
              <dt className="text-muted-foreground">{t('override.setAt')}</dt>
              <dd>{fmt.dateTime(o.set_at)}</dd>
            </div>
          </>
        )}
      </dl>
      {!canConfigure && (
        <p className="mt-3 text-xs text-muted-foreground">{t('override.onlyManagers')}</p>
      )}
      <FormDialog
        open={editing}
        onOpenChange={setEditing}
        title={t('override.dialogTitle')}
        description={t('override.dialogDescription')}
      >
        {editing && (
          <OverrideForm
            establishmentId={establishmentId}
            pass={pass}
            onDone={() => setEditing(false)}
          />
        )}
      </FormDialog>
      <ConfirmDialog
        open={clearing}
        onOpenChange={setClearing}
        title={t('override.clearTitle')}
        description={t('override.clearDescription')}
        confirmLabel={t('override.clear')}
        onConfirm={clear}
        errorText={(e) => problemText(e)}
      />
    </SectionCard>
  );
}

function OverrideForm({
  establishmentId,
  pass,
  onDone,
}: {
  establishmentId: string;
  pass: SeasonPass;
  onDone: () => void;
}) {
  const { t } = useTranslation('passes');
  const problemText = useProblemText();
  const mutation = usePassPriceOverride(establishmentId);
  const form = useForm<OverrideValues>({
    resolver: zodResolver(overrideSchema),
    defaultValues: {
      amount: pass.price_override?.amount ?? pass.total_amount,
      reason: '',
    },
  });
  const submit = form.handleSubmit(async (v) => {
    try {
      await mutation.mutateAsync({
        passId: pass.id,
        version: pass.version,
        body: { amount: v.amount as number, reason: v.reason.trim() },
      });
      toast.success(t('override.saved'));
      onDone();
    } catch {
      // Rendered from mutation.error.
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={submit} className="space-y-5">
        {mutation.error && (
          <Alert variant="destructive">
            <AlertDescription>{problemText(mutation.error)}</AlertDescription>
          </Alert>
        )}
        <FieldRow
          control={form.control}
          name="amount"
          label={t('override.amount')}
          description={t('override.amountHint')}
        >
          {(field) => (
            <MoneyInput
              value={field.value}
              onChange={(v) => field.onChange(v)}
              onBlur={field.onBlur}
            />
          )}
        </FieldRow>
        <FieldRow control={form.control} name="reason" label={t('override.reason')}>
          {(field) => (
            <Textarea rows={3} placeholder={t('override.reasonPlaceholder')} {...field} />
          )}
        </FieldRow>
        <FormActions
          onCancel={onDone}
          pending={mutation.isPending}
          submitLabel={t('override.submit')}
        />
      </form>
    </Form>
  );
}
