import {
  Alert,
  AlertDescription,
  AlertTitle,
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  Spinner,
} from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { CircleCheckIcon, CopyIcon, TriangleAlertIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { Season } from '../configuration/api/resources';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useCopySeason, type SeasonCopyResult } from './api';
import { FormErrorAlert, TextRow } from './kit';
import { copySchema, type CopyValues } from './rules.schema';

/**
 * "Copia stagione" action (spec 8.7): dialog asking the target year and what
 * to copy; dates move keeping the weekday and the new season starts as draft.
 * After the copy the dialog shows the summary and links to the new season.
 */
export function SeasonCopyDialog({
  establishmentId,
  season,
  onOpenSeason,
}: {
  establishmentId: string;
  season: Season;
  onOpenSeason: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  const [open, setOpen] = useState(false);
  const [nonce, setNonce] = useState(0);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button
        variant="outline"
        onClick={() => {
          setNonce((n) => n + 1);
          setOpen(true);
        }}
      >
        <CopyIcon aria-hidden="true" />
        {t('copy.action')}
      </Button>
      <DialogContent closeLabel={t('common.close', { ns: 'translation' })}>
        <DialogHeader>
          <DialogTitle>{t('copy.title', { year: season.year })}</DialogTitle>
          <DialogDescription>{t('copy.description', { year: season.year })}</DialogDescription>
        </DialogHeader>
        <SeasonCopyForm
          key={nonce}
          establishmentId={establishmentId}
          season={season}
          onClose={() => setOpen(false)}
          onOpenSeason={(id) => {
            setOpen(false);
            onOpenSeason(id);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

const INCLUDES = [
  ['include_rate_periods', 'copy.periods'],
  ['include_price_rules', 'copy.rules'],
  ['include_fixed_prices', 'copy.fixed'],
  ['include_policies', 'copy.policies'],
] as const;

export function SeasonCopyForm({
  establishmentId,
  season,
  onClose,
  onOpenSeason,
}: {
  establishmentId: string;
  season: Season;
  onClose: () => void;
  onOpenSeason: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  const copy = useCopySeason(establishmentId);
  const [result, setResult] = useState<SeasonCopyResult | null>(null);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const form = useForm<CopyValues>({
    resolver: zodResolver(copySchema),
    defaultValues: {
      target_year: String(season.year + 1),
      include_rate_periods: true,
      include_price_rules: true,
      include_fixed_prices: true,
      include_policies: true,
    },
  });
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      setResult(
        await copy.mutateAsync({
          seasonId: season.id,
          body: {
            target_year: Number(v.target_year),
            include_rate_periods: v.include_rate_periods,
            include_price_rules: v.include_price_rules,
            include_fixed_prices: v.include_fixed_prices,
            include_policies: v.include_policies,
          },
        }),
      );
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });

  if (result) {
    return (
      <div role="status" className="space-y-4">
        <Alert>
          <CircleCheckIcon aria-hidden="true" />
          <AlertDescription>
            {t('copy.done', {
              year: result.season.year,
              periods: result.rate_periods,
              rules: result.price_rules,
              fixed: result.fixed_prices,
              policies: result.policies,
            })}
          </AlertDescription>
        </Alert>
        {result.warnings.length > 0 && (
          <Alert className="border-warning/50 bg-warning/10">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertTitle>{t('copy.warnings')}</AlertTitle>
            <AlertDescription>
              <ul className="list-inside list-disc">
                {result.warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.close', { ns: 'translation' })}
          </Button>
          <Button onClick={() => onOpenSeason(result.season.id)}>
            {t('copy.open', { year: result.season.year })}
          </Button>
        </DialogFooter>
      </div>
    );
  }
  return (
    <Form {...form}>
      <form noValidate onSubmit={submit} className="space-y-5">
        <FormErrorAlert state={alert} />
        <TextRow
          control={form.control}
          name="target_year"
          label={t('copy.targetYear')}
          inputProps={{ inputMode: 'numeric', className: 'max-w-32' }}
        />
        <fieldset className="space-y-3">
          <legend className="mb-3 text-sm font-medium">{t('copy.include')}</legend>
          {INCLUDES.map(([name, label]) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem className="flex flex-row items-center gap-2">
                  <FormControl>
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={(v) => field.onChange(v === true)}
                    />
                  </FormControl>
                  <FormLabel className="font-normal">{t(label)}</FormLabel>
                </FormItem>
              )}
            />
          ))}
        </fieldset>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </Button>
          <Button type="submit" disabled={copy.isPending}>
            {copy.isPending && <Spinner />}
            {t('copy.submit')}
          </Button>
        </DialogFooter>
      </form>
    </Form>
  );
}
