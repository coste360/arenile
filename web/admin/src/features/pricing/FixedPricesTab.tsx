import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
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
  FormMessage,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  ToggleGroup,
  ToggleGroupItem,
} from '@arenile/shared/ui';
import { EmptyState } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { PackageIcon, PlusIcon, TagsIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { formatCents } from '../configuration/lib/money';
import { fixedPrices, type FixedPrice } from './api';
import {
  DeleteButton,
  EuroTextInput,
  FormErrorAlert,
  OptionSelect,
  QueryState,
  TextRow,
} from './kit';
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
        <p className="text-sm text-muted-foreground">{t('fixed.description')}</p>
        {!p.readOnly && (
          <Button
            onClick={() => {
              setPrefill({ formula });
              editor.create();
            }}
          >
            <PlusIcon aria-hidden="true" />
            {t('fixed.add')}
          </Button>
        )}
      </div>
      <ToggleGroup
        type="single"
        variant="outline"
        aria-label={t('fields.formula')}
        value={formula}
        onValueChange={(v) => v && setFormula(v as FixedFormula)}
        className="flex-wrap"
      >
        {FIXED_FORMULAS.map((f) => (
          <ToggleGroupItem key={f} value={f} className="gap-2">
            {t(`formulas.${f}`)}
            <Badge variant="secondary" className="tabular-nums">
              {countByFormula(f)}
            </Badge>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && p.packs.length === 0 && (
        <EmptyState icon={PackageIcon} title={t('fixed.noPacks')} />
      )}
      {list.data && p.packs.length > 0 && categories.length === 0 && (
        <EmptyState icon={TagsIcon} title={t('fixed.noCategories')} />
      )}
      {list.data &&
        categories.length > 0 &&
        p.packs.map((pack) => (
          <Card key={pack.id} className="gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="text-base">
                {packName(pack.id)}{' '}
                <span className="font-normal text-muted-foreground">
                  · {t(`formulas.${formula}`)}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="px-0">
              <Table>
                <caption className="sr-only">
                  {packName(pack.id)} · {t(`formulas.${formula}`)}
                </caption>
                <TableHeader className="bg-muted/50">
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="pl-6">{t('fields.price_category')}</TableHead>
                    {columns.map((c) => (
                      <TableHead key={c || 'any'} className="text-right">
                        {c || t('fixed.anyPeriod')}
                      </TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {categories.map((cat) => (
                    <TableRow key={cat}>
                      <TableHead scope="row" className="pl-6 font-mono text-xs">
                        {cat}
                      </TableHead>
                      {columns.map((period) => {
                        const f = byCell.get(fixedCellKey(pack.id, formula, cat, period));
                        const where = `${packName(pack.id)} · ${t(`formulas.${formula}`)} · ${cat} · ${period || t('fixed.anyPeriod')}`;
                        return (
                          <TableCell key={period || 'any'} className="p-1 text-right">
                            {p.readOnly ? (
                              <span className="px-2 tabular-nums">
                                {f ? formatCents(f.amount, p.locale) : '—'}
                              </span>
                            ) : (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => open(pack.id, cat, period)}
                                aria-label={
                                  f
                                    ? t('fixed.editAria', { name: where })
                                    : t('fixed.addAria', { name: where })
                                }
                                className={
                                  f
                                    ? 'w-full justify-end tabular-nums'
                                    : 'w-full justify-end text-muted-foreground'
                                }
                              >
                                {f ? (
                                  formatCents(f.amount, p.locale)
                                ) : (
                                  <PlusIcon aria-hidden="true" />
                                )}
                              </Button>
                            )}
                          </TableCell>
                        );
                      })}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        ))}
      <Dialog open={editor.open} onOpenChange={editor.setOpen}>
        <DialogContent
          closeLabel={t('common.close', { ns: 'translation' })}
          className="sm:max-w-lg"
        >
          <DialogHeader>
            <DialogTitle>{editor.item ? t('fixed.editTitle') : t('fixed.add')}</DialogTitle>
            <DialogDescription>{t('fixed.description')}</DialogDescription>
          </DialogHeader>
          <FixedForm
            key={editor.key}
            {...p}
            categories={categories}
            item={editor.item}
            initial={prefill}
            onDone={editor.close}
          />
        </DialogContent>
      </Dialog>
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
                      ...p.periods.map((rp) => ({ value: rp.code, label: rp.code })),
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
        <DialogFooter className="items-center sm:justify-between">
          {p.item ? (
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
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button variant="outline" onClick={p.onDone}>
              {t('common.actions.cancel', { ns: 'translation' })}
            </Button>
            <Button type="submit" disabled={s.pending}>
              {t('common.actions.save', { ns: 'translation' })}
            </Button>
          </div>
        </DialogFooter>
      </form>
    </Form>
  );
}
