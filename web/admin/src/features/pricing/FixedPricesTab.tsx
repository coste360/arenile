import {
  Button,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DeleteButton,
  EmptyState,
  FormActions,
  FormDialog,
  OptionSelect,
  QueryState,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { PackageIcon, PlusIcon, TagsIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { formatCents } from '@arenile/shared/lib/money';
import { fixedPrices, type FixedPrice } from './api';
import { EuroTextInput, FormErrorAlert, TextRow } from './kit';
import type { PricingTabProps } from './RulesTab';
import { localize } from '../configuration/lib/i18nText';
import {
  FIXED_FORMULAS,
  fixedSchema,
  fixedValues,
  toFixedInput,
  type FixedValues,
} from './rules.schema';

type FixedFormula = (typeof FIXED_FORMULAS)[number];

/** One grid row: a price category of a pack. */
interface FixedRow {
  id: string;
  pack: string;
  category: string;
}

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
  const periodName = (code: string) =>
    localize(p.periods.find((rp) => rp.code === code)?.name ?? {}, p.locale, 'it') || code;
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

  const rows: FixedRow[] = p.packs.flatMap((pack) =>
    categories.map((category) => ({ id: `${pack.id}|${category}`, pack: pack.id, category })),
  );
  const tableColumns: ColumnDef<FixedRow>[] = [
    {
      id: 'category',
      meta: { label: t('fields.price_category') },
      header: t('fields.price_category'),
      cell: ({ row }) => (
        <>
          <span className="block font-medium">
            {p.categoryNames[row.original.category] ?? row.original.category}
          </span>
          <span className="font-mono text-xs text-muted-foreground">{row.original.category}</span>
        </>
      ),
    },
    ...columns.map((period): ColumnDef<FixedRow> => ({
      id: `period-${period || 'any'}`,
      meta: {
        label: period ? periodName(period) : t('fixed.anyPeriod'),
        align: 'right',
      },
      header: period ? periodName(period) : t('fixed.anyPeriod'),
      cell: ({ row }) => {
        const { pack, category } = row.original;
        const f = byCell.get(fixedCellKey(pack, formula, category, period));
        if (p.readOnly) return f ? formatCents(f.amount) : '—';
        const where = `${packName(pack)} · ${t(`formulas.${formula}`)} · ${p.categoryNames[category] ?? category} · ${period ? periodName(period) : t('fixed.anyPeriod')}`;
        return (
          <Button
            variant="ghost"
            size="sm"
            className="-mr-3"
            onClick={() => open(pack, category, period)}
            aria-label={
              f ? t('fixed.editAria', { name: where }) : t('fixed.addAria', { name: where })
            }
          >
            {f ? (
              formatCents(f.amount)
            ) : (
              <PlusIcon aria-hidden="true" className="text-muted-foreground" />
            )}
          </Button>
        );
      },
    })),
  ];

  const add = !p.readOnly && (
    <Button
      onClick={() => {
        setPrefill({ formula });
        editor.create();
      }}
    >
      <PlusIcon aria-hidden="true" />
      {t('fixed.add')}
    </Button>
  );

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('fixed.description')}</p>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && p.packs.length === 0 && (
        <EmptyState icon={PackageIcon} title={t('fixed.noPacks')} />
      )}
      {list.data && p.packs.length > 0 && categories.length === 0 && (
        <EmptyState icon={TagsIcon} title={t('fixed.noCategories')} action={add || undefined} />
      )}
      {list.data && rows.length > 0 && (
        <DataTable
          caption={t('fixed.caption', { formula: t(`formulas.${formula}`) })}
          columns={tableColumns}
          data={rows}
          getRowId={(r) => r.id}
          pageSize={0}
          columnToggle={false}
          filters={
            <SegmentedControl
              collapseOnNarrow
              label={t('fields.formula')}
              value={formula}
              onValueChange={(v) => setFormula(v as FixedFormula)}
              options={FIXED_FORMULAS.map((f) => ({
                value: f,
                label: (
                  <>
                    {t(`formulas.${f}`)}
                    <span className="text-muted-foreground tabular-nums">{countByFormula(f)}</span>
                  </>
                ),
              }))}
            />
          }
          toolbar={add}
          groupBy={(r) => r.pack}
          renderGroup={(pack) => (
            <>
              {packName(pack)}{' '}
              <span className="font-normal text-muted-foreground">
                · {t(`formulas.${formula}`)}
              </span>
            </>
          )}
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('fixed.editTitle') : t('fixed.add')}
        description={t('fixed.description')}
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
    defaultValues: { ...fixedValues(p.item), ...(p.item ? {} : p.initial) },
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
  const { control } = form;
  return (
    <Form {...form}>
      <form noValidate onSubmit={s.submit} className="space-y-4">
        <FormErrorAlert state={s.alert} onReload={p.onDone} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={control}
            name="pack_id"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('fields.pack')}</FormLabel>
                <FormControl>
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    placeholder={t('choose')}
                    options={p.packs.map((pk) => ({
                      value: pk.id,
                      label: p.name(pk.name) || pk.code,
                    }))}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name="formula"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('fields.formula')}</FormLabel>
                <FormControl>
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    options={FIXED_FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) }))}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <TextRow
            control={control}
            name="price_category"
            label={t('fields.price_category')}
            inputProps={{ list: 'fixed-category-options', autoComplete: 'off' }}
          />
          <datalist id="fixed-category-options">
            {p.categories.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
          <FormField
            control={control}
            name="rate_period_code"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('fields.rate_period')}</FormLabel>
                <FormControl>
                  <OptionSelect
                    value={field.value}
                    onChange={field.onChange}
                    options={[
                      { value: '', label: t('fixed.anyPeriod') },
                      ...p.periods.map((rp) => ({
                        value: rp.code,
                        label: localize(rp.name, p.locale, 'it') || rp.code,
                      })),
                    ]}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name="amount"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('fields.amount')}</FormLabel>
                <FormControl>
                  <EuroTextInput
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
        <FormActions
          onCancel={p.onDone}
          pending={s.pending}
          start={
            p.item && (
              <DeleteButton
                withText
                label={t('fixed.deleteAria', { name: p.item.price_category })}
                message={t('fixed.deleteConfirm', { name: p.item.price_category })}
                onConfirm={async () => {
                  if (!p.item) return;
                  await remove.mutateAsync({ id: p.item.id, version: p.item.version });
                  p.onDone();
                }}
              />
            )
          }
        />
      </form>
    </Form>
  );
}
