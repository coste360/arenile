import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../configuration/components/dialogs';
import { Field, Select } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { formatCents } from '../configuration/lib/money';
import { fixedPrices, type FixedPrice } from './api';
import type { PricingTabProps } from './RulesTab';
import {
  FIXED_FORMULAS,
  fixedSchema,
  fixedValues,
  toFixedInput,
  type FixedValues,
} from './rules.schema';

type FixedFormula = (typeof FIXED_FORMULAS)[number];

/** Key of a grid cell: pack × formula × price category × rate period ("" = any period). */
function fixedCellKey(pack: string, formula: string, category: string, period: string) {
  return `${pack}|${formula}|${category}|${period}`;
}

/**
 * Fixed-duration prices (week, fortnight, month, season) as a grid: for the
 * chosen formula, one table per pack with price categories as rows and rate
 * periods as columns (the first column is "every period"). A cell opens the
 * editor of that price, or a prefilled new one when it is empty.
 */
export function FixedPricesTab(p: PricingTabProps & { categories: string[] }) {
  const { t } = useTranslation('pricing');
  const list = fixedPrices.useList(p.establishmentId, { season_id: p.seasonId });
  const editor = useEditor<FixedPrice>();
  const [formula, setFormula] = useState<FixedFormula>('week');
  const [prefill, setPrefill] = useState<Partial<FixedValues>>();
  const items = list.data ?? [];
  const byCell = new Map(
    items.map((f) => [
      fixedCellKey(f.pack_id, f.formula, f.price_category, f.rate_period_code ?? ''),
      f,
    ]),
  );
  const categories = [...new Set([...p.categories, ...items.map((f) => f.price_category)])].sort();
  const columns = ['', ...p.periods.map((rp) => rp.code)];
  const countByFormula = (f: FixedFormula) => items.filter((x) => x.formula === f).length;
  const packName = (id: string) => {
    const x = p.packs.find((pk) => pk.id === id);
    return x ? p.name(x.name) || x.code : id.slice(0, 8);
  };
  const open = (pack: string, category: string, period: string) => {
    const existing = byCell.get(fixedCellKey(pack, formula, category, period));
    if (existing) {
      editor.edit(existing);
      return;
    }
    setPrefill({ pack_id: pack, formula, price_category: category, rate_period_code: period });
    editor.create();
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-slate-600">{t('fixed.description')}</p>
        {!p.readOnly && (
          <Button
            onClick={() => {
              setPrefill({ formula });
              editor.create();
            }}
          >
            {t('fixed.add')}
          </Button>
        )}
      </div>
      <div role="radiogroup" aria-label={t('fields.formula')} className="flex flex-wrap gap-1">
        {FIXED_FORMULAS.map((f) => (
          <button
            key={f}
            type="button"
            role="radio"
            aria-checked={formula === f}
            onClick={() => setFormula(f)}
            className={
              formula === f
                ? 'rounded-md bg-slate-900 px-3 py-1.5 text-sm font-medium text-white'
                : 'rounded-md px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100'
            }
          >
            {t(`formulas.${f}`)}{' '}
            <span className="tabular-nums opacity-70">({countByFormula(f)})</span>
          </button>
        ))}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && p.packs.length === 0 && <EmptyState>{t('fixed.noPacks')}</EmptyState>}
      {list.data && p.packs.length > 0 && categories.length === 0 && (
        <EmptyState>{t('fixed.noCategories')}</EmptyState>
      )}
      {list.data &&
        categories.length > 0 &&
        p.packs.map((pack) => (
          <Card key={pack.id} className="overflow-x-auto">
            <table className="w-full min-w-[32rem] text-sm">
              <caption className="px-3 pt-3 text-left font-semibold">
                {packName(pack.id)} · {t(`formulas.${formula}`)}
              </caption>
              <thead className="text-left text-slate-600">
                <tr>
                  <th scope="col" className="px-3 py-2 font-medium">
                    {t('fields.price_category')}
                  </th>
                  {columns.map((c) => (
                    <th key={c || 'any'} scope="col" className="px-3 py-2 text-right font-medium">
                      {c || t('fixed.anyPeriod')}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {categories.map((cat) => (
                  <tr key={cat} className="border-t border-slate-100">
                    <th scope="row" className="px-3 py-2 text-left font-mono text-xs font-medium">
                      {cat}
                    </th>
                    {columns.map((period) => {
                      const f = byCell.get(fixedCellKey(pack.id, formula, cat, period));
                      const where = `${packName(pack.id)} · ${t(`formulas.${formula}`)} · ${cat} · ${period || t('fixed.anyPeriod')}`;
                      return (
                        <td key={period || 'any'} className="px-1 py-1 text-right">
                          {p.readOnly ? (
                            <span className="px-2 tabular-nums">
                              {f ? formatCents(f.amount, p.locale) : '—'}
                            </span>
                          ) : (
                            <button
                              type="button"
                              onClick={() => open(pack.id, cat, period)}
                              aria-label={
                                f
                                  ? t('fixed.editAria', { name: where })
                                  : t('fixed.addAria', { name: where })
                              }
                              className={`w-full rounded px-2 py-1 text-right tabular-nums hover:bg-slate-100 ${f ? '' : 'text-slate-400'}`}
                            >
                              {f ? formatCents(f.amount, p.locale) : '+'}
                            </button>
                          )}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        ))}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('fixed.editTitle') : t('fixed.add')}
      >
        <FixedForm
          key={editor.key}
          {...p}
          categories={categories}
          item={editor.item}
          initial={prefill}
          onDone={editor.close}
        />
      </FormDialog>
    </div>
  );
}

function FixedForm(
  p: PricingTabProps & {
    categories: string[];
    item?: FixedPrice;
    initial?: Partial<FixedValues>;
    onDone: () => void;
  },
) {
  const { t } = useTranslation('pricing');
  const form = useForm<FixedValues>({
    resolver: zodResolver(fixedSchema),
    defaultValues: { ...fixedValues(p.item, p.locale), ...(p.item ? {} : p.initial) },
  });
  const remove = fixedPrices.useRemove(p.establishmentId);
  const create = fixedPrices.useCreate(p.establishmentId);
  const update = fixedPrices.useUpdate(p.establishmentId);
  const s = useCrudSubmit({
    form,
    item: p.item,
    create,
    update,
    toInput: (v: FixedValues) => toFixedInput(p.seasonId, v),
    onDone: p.onDone,
  });
  const {
    register,
    formState: { errors },
  } = form;
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={p.onDone} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="fixed-pack" label={t('fields.pack')} error={errors.pack_id}>
          <Select {...fieldAria('fixed-pack', errors.pack_id)} {...register('pack_id')}>
            <option value="">{t('choose')}</option>
            {p.packs.map((pk) => (
              <option key={pk.id} value={pk.id}>
                {p.name(pk.name) || pk.code}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="fixed-formula" label={t('fields.formula')} error={errors.formula}>
          <Select {...fieldAria('fixed-formula', errors.formula)} {...register('formula')}>
            {FIXED_FORMULAS.map((f) => (
              <option key={f} value={f}>
                {t(`formulas.${f}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="fixed-category" label={t('fields.price_category')} error={errors.price_category}>
          <Input
            {...fieldAria('fixed-category', errors.price_category)}
            list="fixed-category-options"
            {...register('price_category')}
          />
          <datalist id="fixed-category-options">
            {p.categories.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
        </Field>
        <Field id="fixed-period" label={t('fields.rate_period')} error={errors.rate_period_code}>
          <Select
            {...fieldAria('fixed-period', errors.rate_period_code)}
            {...register('rate_period_code')}
          >
            <option value="">{t('fixed.anyPeriod')}</option>
            {p.periods.map((rp) => (
              <option key={rp.id} value={rp.code}>
                {rp.code}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="fixed-amount" label={t('fields.amount')} error={errors.amount}>
          <Input
            {...fieldAria('fixed-amount', errors.amount)}
            inputMode="decimal"
            {...register('amount')}
          />
        </Field>
      </div>
      <div className="flex items-center justify-between gap-2">
        {p.item ? (
          <ConfirmDelete
            label={t('fixed.deleteAria', { name: p.item.price_category })}
            message={t('fixed.deleteConfirm', { name: p.item.price_category })}
            onConfirm={async () => {
              if (!p.item) return;
              await remove.mutateAsync({ id: p.item.id, version: p.item.version });
              p.onDone();
            }}
          />
        ) : (
          <span />
        )}
        <FormActions onCancel={p.onDone} pending={s.pending} />
      </div>
    </form>
  );
}
