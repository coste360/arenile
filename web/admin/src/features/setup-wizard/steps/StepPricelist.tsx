import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { FieldDescription } from '@arenile/shared/ui/field';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { parseEuroToCents, formatCents } from '@arenile/shared/lib/money';
import { MoneyInput } from '@arenile/shared/ui/patterns';
import { CircleCheckIcon, TriangleAlertIcon } from 'lucide-react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useController, useForm, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ratePeriods, seasons, zones, type RatePeriod } from '../../configuration/api/resources';
import { FieldError } from '../../configuration/components/fields';
import { FormAlert } from '../../configuration/components/FormAlert';
import { errorAt, fieldAria } from '../../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../../configuration/lib/formErrors';
import { localize } from '../../configuration/lib/i18nText';
import { useQuickPricelist, type QuickPricelistResult } from '../api';
import {
  emptyGrid,
  pricelistSchema,
  toQuickPricelistRequest,
  type PricelistValues,
} from '../wizard.schema';
import type { StepProps } from './types';

/**
 * Step 4: quick price list — daily price per price category and rate period.
 * The rows are the categories the server requires a price for: those of the
 * zones plus the spot overrides, as reported by the setup status
 * (`price_categories`); zones alone would miss the overrides.
 */
export function StepPricelist(props: StepProps) {
  const { t } = useTranslation('setupWizard');
  const { ctx, progress, status } = props;
  const seasonList = seasons.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const season =
    seasonList.data?.find((s) => s.id === progress.seasonId) ??
    seasonList.data?.find((s) => s.status === 'active') ??
    [...(seasonList.data ?? [])].sort((a, b) => b.year - a.year)[0];
  const periodList = ratePeriods.useList(
    ctx.establishmentId,
    { season_id: season?.id },
    { enabled: !!season },
  );

  if (seasonList.isPending || zoneList.isPending || (season && periodList.isPending)) {
    return (
      <div role="status" className="space-y-3">
        <span className="sr-only">{t('loading')}</span>
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  const categories = [
    ...new Set([
      ...[...(zoneList.data ?? [])]
        .sort((a, b) => a.sort_order - b.sort_order)
        .map((z) => z.price_category),
      ...(status?.price_categories ?? []),
    ]),
  ];
  const periods = [...(periodList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  if (!season || categories.length === 0 || periods.length === 0) {
    return (
      <div className="space-y-3">
        <Alert role="status" className="border-warning/50 bg-warning/10">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription className="text-foreground">
            {!season
              ? t('pricelist.noSeason')
              : categories.length === 0
                ? t('pricelist.noZones')
                : t('pricelist.noPeriods')}
          </AlertDescription>
        </Alert>
      </div>
    );
  }
  return (
    <PricelistGrid
      {...props}
      key={`${season.id}-${categories.join()}-${periods.map((p) => p.code).join()}`}
      seasonId={season.id}
      seasonYear={season.year}
      categories={categories}
      periods={periods}
    />
  );
}

function PricelistGrid({
  ctx,
  dispatch,
  seasonId,
  seasonYear,
  categories,
  periods,
}: StepProps & {
  seasonId: string;
  seasonYear: number;
  categories: string[];
  periods: RatePeriod[];
}) {
  const { t } = useTranslation('setupWizard');
  const quick = useQuickPricelist(ctx.establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [result, setResult] = useState<QuickPricelistResult | null>(null);
  const form = useForm({
    resolver: zodResolver(pricelistSchema),
    defaultValues: emptyGrid(categories.length, periods.length),
  });
  const {
    formState: { errors },
  } = form;
  const periodName = (p: RatePeriod) => localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.code;
  const currency = ctx.establishment?.currency ?? 'EUR';

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const req = toQuickPricelistRequest(
        seasonId,
        categories,
        periods.map((p) => p.code),
        values,
      );
      setResult(await quick.mutateAsync(req));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });

  if (result) {
    return (
      <div className="space-y-4">
        <Alert role="status" className="border-success/40 bg-success/10">
          <CircleCheckIcon aria-hidden="true" className="text-success" />
          <AlertDescription className="text-foreground">
            {t('pricelist.created', { count: result.rules.length, replaced: result.replaced })}
          </AlertDescription>
        </Alert>
        <Button onClick={() => dispatch({ type: 'complete' })}>{t('continue')}</Button>
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      <p className="text-sm text-muted-foreground">{t('pricelist.intro', { year: seasonYear })}</p>
      <FormAlert state={alert} />
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableCaption className="sr-only">{t('pricelist.caption', { currency })}</TableCaption>
          <TableHeader className="bg-muted/50">
            <TableRow className="hover:bg-transparent">
              <TableHead scope="col">{t('pricelist.category')}</TableHead>
              {periods.map((p) => (
                <TableHead key={p.id} scope="col">
                  <span className="inline-flex items-center gap-1.5">
                    <svg viewBox="0 0 12 12" aria-hidden="true" className="size-3">
                      <rect width="12" height="12" rx="3" fill={p.color} />
                    </svg>
                    {periodName(p)}
                  </span>
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {categories.map((cat, i) => (
              <TableRow key={cat}>
                <TableHead scope="row" className="font-mono text-xs">
                  {cat}
                </TableHead>
                {periods.map((p, j) => (
                  <TableCell key={p.id} className="align-top">
                    <PriceCell
                      control={form.control}
                      row={i}
                      col={j}
                      label={t('pricelist.cellAria', { category: cat, period: periodName(p) })}
                    />
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <FieldError id="pl-cells" error={errorAt(errors, 'cells')} />
      <FieldDescription>
        {t('pricelist.hint', { example: formatCents(2500, currency) })}
      </FieldDescription>
      <Button type="submit" disabled={quick.isPending}>
        {quick.isPending && <Spinner />}
        {quick.isPending ? t('pricelist.generating') : t('pricelist.generate')}
      </Button>
    </form>
  );
}

/**
 * Shared MoneyInput in a grid cell. The form keeps the text as typed (so an
 * invalid amount stays flagged after blur) and the zod schema parses it to
 * integer cents with the same exact parser.
 */
function PriceCell({
  control,
  row,
  col,
  label,
}: {
  control: Control<PricelistValues>;
  row: number;
  col: number;
  label: string;
}) {
  const {
    field: { ref, ...field },
    fieldState,
  } = useController({ control, name: `cells.${row}.${col}` });
  const id = `pl-${row}-${col}`;
  return (
    <div className="space-y-1">
      <MoneyInput
        {...fieldAria(id, fieldState.error)}
        ref={ref}
        aria-label={label}
        placeholder="0,00"
        className="w-32"
        value={field.value.trim() === '' ? null : parseEuroToCents(field.value)}
        onChange={() => {}}
        onInput={(e) => field.onChange(e.currentTarget.value)}
        onBlur={field.onBlur}
      />
      <FieldError id={id} error={fieldState.error} />
    </div>
  );
}
