import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { Badge, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../configuration/components/dialogs';
import { Checkbox, Field, Textarea } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { agreements, promoCodes, type Agreement, type PromoCode } from './api';
import { ActionFields, ConditionFields } from './components';
import { describeAction } from './format';
import type { PricingTabProps } from './RulesTab';
import {
  ACTIONS_BY_STAGE,
  FORM_ALIASES,
  agreementSchema,
  agreementValues,
  promoSchema,
  promoValues,
  toAgreementInput,
  toPromoInput,
  type AgreementValues,
  type PromoValues,
} from './rules.schema';

function validity(
  t: (k: string, o?: Record<string, unknown>) => string,
  from?: string,
  to?: string,
) {
  if (from && to) return t('validity.range', { from, to });
  if (from) return t('validity.from', { from });
  if (to) return t('validity.to', { to });
  return t('validity.always');
}

/** Promo codes (case-insensitive, unique) and agreements: discount-stage actions (spec 8.4). */
export function DiscountsTab(p: PricingTabProps) {
  return (
    <div className="space-y-8">
      <PromoSection {...p} />
      <AgreementSection {...p} />
    </div>
  );
}

function PromoSection(p: PricingTabProps) {
  const { t } = useTranslation('pricing');
  const list = promoCodes.useList(p.establishmentId);
  const remove = promoCodes.useRemove(p.establishmentId);
  const editor = useEditor<PromoCode>();
  const items = [...(list.data ?? [])].sort((a, b) => a.code.localeCompare(b.code));
  return (
    <section aria-labelledby="promo-title" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 id="promo-title" className="text-lg font-semibold">
            {t('promo.title')}
          </h2>
          <p className="text-sm text-slate-600">{t('promo.description')}</p>
        </div>
        {!p.readOnly && <Button onClick={editor.create}>{t('promo.add')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('promo.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-sm">
            <caption className="sr-only">{t('promo.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.code')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.action')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.validity')}
                </th>
                <th scope="col" className="px-3 py-2 text-right font-medium">
                  {t('promo.uses')}
                </th>
                <th scope="col" className="px-3 py-2">
                  <span className="sr-only">{t('actionsColumn')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((x) => (
                <tr key={x.id} className="border-t border-slate-100">
                  <th scope="row" className="px-3 py-2 text-left font-mono font-medium">
                    {x.code}
                    {!x.active && (
                      <span className="ml-2 font-sans">
                        <Badge>{t('inactive')}</Badge>
                      </span>
                    )}
                  </th>
                  <td className="px-3 py-2">{describeAction(t, x.action, p.locale)}</td>
                  <td className="px-3 py-2">{validity(t, x.valid_from, x.valid_to)}</td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {x.max_uses ? `${x.uses} / ${x.max_uses}` : x.uses}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {!p.readOnly && (
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => editor.edit(x)}
                          aria-label={t('promo.editAria', { name: x.code })}
                        >
                          {t('common.actions.edit', { ns: 'translation' })}
                        </Button>
                        <ConfirmDelete
                          label={t('promo.deleteAria', { name: x.code })}
                          message={t('promo.deleteConfirm', { name: x.code })}
                          onConfirm={() => remove.mutateAsync({ id: x.id, version: x.version })}
                        />
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('promo.editTitle', { name: editor.item.code }) : t('promo.add')}
      >
        <PromoForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </section>
  );
}

function PromoForm(p: PricingTabProps & { item?: PromoCode; onDone: () => void }) {
  const { t } = useTranslation('pricing');
  const form = useForm<PromoValues>({
    resolver: zodResolver(promoSchema),
    defaultValues: promoValues(p.item, p.locale),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: promoCodes.useCreate(p.establishmentId),
    update: promoCodes.useUpdate(p.establishmentId),
    toInput: toPromoInput,
    onDone: p.onDone,
    aliases: FORM_ALIASES,
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const text = (name: keyof PromoValues, label: string, type = 'text') => (
    <Field id={`promo-${name}`} label={label} error={errors[name] as never}>
      <Input
        {...fieldAria(`promo-${name}`, errors[name] as never)}
        type={type}
        {...register(name)}
      />
    </Field>
  );
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={p.onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        {text('code', t('fields.code'))}
        {text('valid_from', t('fields.valid_from'), 'date')}
        {text('valid_to', t('fields.valid_to'), 'date')}
        {text('max_uses', t('fields.max_uses'))}
        {text('max_uses_per_customer', t('fields.max_uses_per_customer'))}
      </div>
      <Field id="promo-description" label={t('fields.description')} error={errors.description}>
        <Textarea
          {...fieldAria('promo-description', errors.description)}
          {...register('description')}
        />
      </Field>
      <ActionFields
        control={control}
        register={register}
        errors={errors}
        allowed={ACTIONS_BY_STAGE.discount}
        idPrefix="promo"
      />
      <ConditionFields
        control={control}
        register={register}
        errors={errors}
        periods={p.periods}
        zones={p.zones}
        idPrefix="promo"
      />
      <div className="flex flex-wrap gap-6">
        <Checkbox id="promo-stackable" label={t('fields.stackable')} {...register('stackable')} />
        <Checkbox id="promo-active" label={t('fields.active')} {...register('active')} />
      </div>
      <FormActions onCancel={p.onDone} pending={s.pending} />
    </form>
  );
}

function AgreementSection(p: PricingTabProps) {
  const { t } = useTranslation('pricing');
  const list = agreements.useList(p.establishmentId);
  const remove = agreements.useRemove(p.establishmentId);
  const editor = useEditor<Agreement>();
  const items = [...(list.data ?? [])].sort((a, b) => a.name.localeCompare(b.name));
  return (
    <section aria-labelledby="agreements-title" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 id="agreements-title" className="text-lg font-semibold">
            {t('agreements.title')}
          </h2>
          <p className="text-sm text-slate-600">{t('agreements.description')}</p>
        </div>
        {!p.readOnly && <Button onClick={editor.create}>{t('agreements.add')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('agreements.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-sm">
            <caption className="sr-only">{t('agreements.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.name')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.partner_name')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.action')}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t('fields.validity')}
                </th>
                <th scope="col" className="px-3 py-2">
                  <span className="sr-only">{t('actionsColumn')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((x) => (
                <tr key={x.id} className="border-t border-slate-100">
                  <th scope="row" className="px-3 py-2 text-left font-medium">
                    {x.name}
                    {x.code && (
                      <span className="ml-2 font-mono text-xs text-slate-500">{x.code}</span>
                    )}
                    {!x.active && (
                      <span className="ml-2">
                        <Badge>{t('inactive')}</Badge>
                      </span>
                    )}
                  </th>
                  <td className="px-3 py-2">{x.partner_name || '—'}</td>
                  <td className="px-3 py-2">{describeAction(t, x.action, p.locale)}</td>
                  <td className="px-3 py-2">{validity(t, x.valid_from, x.valid_to)}</td>
                  <td className="px-3 py-2 text-right">
                    {!p.readOnly && (
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => editor.edit(x)}
                          aria-label={t('agreements.editAria', { name: x.name })}
                        >
                          {t('common.actions.edit', { ns: 'translation' })}
                        </Button>
                        <ConfirmDelete
                          label={t('agreements.deleteAria', { name: x.name })}
                          message={t('agreements.deleteConfirm', { name: x.name })}
                          onConfirm={() => remove.mutateAsync({ id: x.id, version: x.version })}
                        />
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={
          editor.item ? t('agreements.editTitle', { name: editor.item.name }) : t('agreements.add')
        }
      >
        <AgreementForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </section>
  );
}

function AgreementForm(p: PricingTabProps & { item?: Agreement; onDone: () => void }) {
  const { t } = useTranslation('pricing');
  const form = useForm<AgreementValues>({
    resolver: zodResolver(agreementSchema),
    defaultValues: agreementValues(p.item, p.locale),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: agreements.useCreate(p.establishmentId),
    update: agreements.useUpdate(p.establishmentId),
    toInput: toAgreementInput,
    onDone: p.onDone,
    aliases: FORM_ALIASES,
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const text = (name: keyof AgreementValues, label: string, type = 'text', hint?: string) => (
    <Field id={`agr-${name}`} label={label} error={errors[name] as never} hint={hint}>
      <Input
        {...fieldAria(`agr-${name}`, errors[name] as never, !!hint)}
        type={type}
        {...register(name)}
      />
    </Field>
  );
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={p.onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        {text('name', t('fields.name'))}
        {text('partner_name', t('fields.partner_name'))}
        {text('code', t('fields.code'), 'text', t('hints.agreementCode'))}
        {text('valid_from', t('fields.valid_from'), 'date')}
        {text('valid_to', t('fields.valid_to'), 'date')}
      </div>
      <Field id="agr-notes" label={t('fields.notes')} error={errors.notes}>
        <Textarea {...fieldAria('agr-notes', errors.notes)} {...register('notes')} />
      </Field>
      <ActionFields
        control={control}
        register={register}
        errors={errors}
        allowed={ACTIONS_BY_STAGE.discount}
        idPrefix="agr"
      />
      <ConditionFields
        control={control}
        register={register}
        errors={errors}
        periods={p.periods}
        zones={p.zones}
        idPrefix="agr"
      />
      <div className="flex flex-wrap gap-6">
        <Checkbox id="agr-stackable" label={t('fields.stackable')} {...register('stackable')} />
        <Checkbox id="agr-active" label={t('fields.active')} {...register('active')} />
      </div>
      <FormActions onCancel={p.onDone} pending={s.pending} />
    </form>
  );
}
