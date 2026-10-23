import { problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { Textarea } from '@arenile/shared/ui/textarea';
import { FormActions, FormDialog, MoneyInput } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { Controller, useForm, useWatch, type FieldError } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCloseSession, type CashSession, type CashSessionDetail } from '../api';
import { closeSessionSchema, type CloseSessionValues } from '../schemas';
import {
  difference,
  expectedTotals,
  methodsShown,
  sumTotals,
  toCountedTotals,
} from '../lib/totals';
import type { CashContext } from '../useCashContext';
import { DifferenceAmount } from './Difference';
import { fieldAria } from '../lib/aria';
import { CashField } from './fields';

/** Closing with the counted money per method, live difference against the expected (INV-25). */
export function CloseSessionDialog({
  ctx,
  detail,
  open,
  onOpenChange,
  onClosed,
}: {
  ctx: Pick<CashContext, 'establishmentId' | 'money'>;
  detail: CashSessionDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onClosed: (session: CashSession) => void;
}) {
  const { t } = useTranslation('cash');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('close.title')}
      description={t('close.description')}
      wide
    >
      {open && (
        <CloseForm
          ctx={ctx}
          detail={detail}
          onCancel={() => onOpenChange(false)}
          onClosed={(s) => {
            onOpenChange(false);
            onClosed(s);
          }}
        />
      )}
    </FormDialog>
  );
}

function CloseForm({
  ctx,
  detail,
  onCancel,
  onClosed,
}: {
  ctx: Pick<CashContext, 'establishmentId' | 'money'>;
  detail: CashSessionDetail;
  onCancel: () => void;
  onClosed: (session: CashSession) => void;
}) {
  const { t } = useTranslation('cash');
  const expected = expectedTotals(detail);
  const methods = methodsShown(expected);
  const close = useCloseSession(ctx.establishmentId, detail.session);
  // Cash must be counted; electronic methods default to the expected amount.
  const defaults: Record<string, number | null> = {};
  for (const m of methods) defaults[m] = m === 'cash' ? null : (expected[m] ?? 0);
  const form = useForm<CloseSessionValues>({
    resolver: zodResolver(closeSessionSchema),
    defaultValues: { counted: defaults, notes: '' },
  });
  const counted = useWatch({ control: form.control, name: 'counted' });
  const countedTotals = toCountedTotals(counted, methods);
  const diff = difference(countedTotals, expected);
  const countedErrors = form.formState.errors.counted as
    Record<string, FieldError | undefined> | undefined;

  const submit = form.handleSubmit(async (v) => {
    try {
      const session = await close.mutateAsync({
        counted_totals: toCountedTotals(v.counted, methods),
        ...(v.notes.trim() ? { notes: v.notes.trim() } : {}),
      });
      toast.success(t('close.closed'));
      onClosed(session);
    } catch {
      // Rendered from close.error (412: reload, someone else changed the session).
    }
  });

  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      {close.error && (
        <Alert variant="destructive">
          <AlertDescription>
            {t(problemMessageKey(close.error), { ns: 'translation' })}
          </AlertDescription>
        </Alert>
      )}
      <Table>
        <caption className="sr-only">{t('close.title')}</caption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('close.method')}</TableHead>
            <TableHead className="text-right">{t('close.expected')}</TableHead>
            <TableHead className="text-right">{t('close.counted')}</TableHead>
            <TableHead className="text-right">{t('close.difference')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {methods.map((m) => {
            const id = `close-counted-${m}`;
            const err = countedErrors?.[m];
            return (
              <TableRow key={m}>
                <TableCell className="font-medium">{t(`method.${m}`)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {ctx.money(expected[m] ?? 0)}
                </TableCell>
                <TableCell className="text-right">
                  <CashField
                    id={id}
                    label={
                      <span className="sr-only">
                        {t('close.countedFor', { method: t(`method.${m}`) })}
                      </span>
                    }
                    error={err}
                    className="items-end"
                  >
                    <Controller
                      control={form.control}
                      name={`counted.${m}`}
                      render={({ field }) => (
                        <MoneyInput
                          {...fieldAria(id, err)}
                          className="ml-auto"
                          value={field.value ?? null}
                          onChange={(v) => field.onChange(v)}
                          onBlur={field.onBlur}
                        />
                      )}
                    />
                  </CashField>
                </TableCell>
                <TableCell className="text-right" aria-live="polite">
                  {countedTotals[m] === undefined ? (
                    <span className="text-muted-foreground">—</span>
                  ) : (
                    <DifferenceAmount
                      cents={diff.byMethod[m] ?? 0}
                      money={ctx.money}
                      badge={false}
                    />
                  )}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell>{t('close.total')}</TableCell>
            <TableCell className="text-right tabular-nums">
              {ctx.money(sumTotals(expected))}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {ctx.money(sumTotals(countedTotals))}
            </TableCell>
            <TableCell
              className="text-right"
              aria-live="polite"
              data-testid="close-total-difference"
            >
              <DifferenceAmount cents={diff.total} money={ctx.money} />
            </TableCell>
          </TableRow>
        </TableFooter>
      </Table>
      <CashField id="close-notes" label={t('fields.notes')} error={form.formState.errors.notes}>
        <Textarea
          {...fieldAria('close-notes', form.formState.errors.notes)}
          rows={2}
          placeholder={t('close.notesPlaceholder')}
          {...form.register('notes')}
        />
      </CashField>
      <FormActions onCancel={onCancel} pending={close.isPending} submitLabel={t('close.submit')} />
    </form>
  );
}
