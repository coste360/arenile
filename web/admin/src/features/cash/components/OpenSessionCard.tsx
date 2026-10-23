import { problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import { MoneyInput } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { LockOpenIcon } from 'lucide-react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { MAIN_REGISTER, useOpenSession } from '../api';
import { openSessionSchema, type OpenSessionValues } from '../schemas';
import { fieldAria } from '../lib/aria';
import { CashField } from './fields';

/** Shown when the register has no open session: count the float and open. */
export function OpenSessionCard({ establishmentId }: { establishmentId: string }) {
  const { t } = useTranslation('cash');
  const open = useOpenSession(establishmentId);
  const form = useForm<OpenSessionValues>({
    resolver: zodResolver(openSessionSchema),
    defaultValues: { opening_float: null, notes: '' },
  });
  const submit = form.handleSubmit(async (v) => {
    try {
      await open.mutateAsync({
        register: MAIN_REGISTER,
        opening_float: v.opening_float as number,
        ...(v.notes.trim() ? { notes: v.notes.trim() } : {}),
      });
      toast.success(t('open.opened'));
    } catch {
      // Rendered below from open.error.
    }
  });
  const errors = form.formState.errors;
  return (
    <Card className="max-w-xl">
      <form noValidate onSubmit={submit} aria-labelledby="open-session-title">
        <CardHeader>
          <CardTitle id="open-session-title" className="flex items-center gap-2">
            <LockOpenIcon aria-hidden="true" className="size-5" />
            {t('open.title')}
          </CardTitle>
          <CardDescription>{t('open.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5 pt-5">
          {open.error && (
            <Alert variant="destructive">
              <AlertDescription>
                {t(problemMessageKey(open.error), { ns: 'translation' })}
              </AlertDescription>
            </Alert>
          )}
          <CashField
            id="open-float"
            label={t('open.float')}
            hint={t('open.floatHint')}
            error={errors.opening_float}
          >
            <Controller
              control={form.control}
              name="opening_float"
              render={({ field }) => (
                <MoneyInput
                  {...fieldAria('open-float', errors.opening_float, true)}
                  value={field.value}
                  onChange={(v) => field.onChange(v)}
                  onBlur={field.onBlur}
                />
              )}
            />
          </CashField>
          <CashField id="open-notes" label={t('fields.notes')} error={errors.notes}>
            <Textarea
              {...fieldAria('open-notes', errors.notes)}
              rows={2}
              {...form.register('notes')}
            />
          </CashField>
        </CardContent>
        <CardFooter className="justify-end pt-5">
          <Button type="submit" disabled={open.isPending}>
            {open.isPending && <Spinner />}
            {t('open.submit')}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}
