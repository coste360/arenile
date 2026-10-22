import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { closureDays, type ClosureDay } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { Badge, ConfigPage, EmptyState, QueryState } from '../components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Checkbox, CheckboxGroup, Field, Select } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { fieldAria } from '../lib/aria';
import { formatDate } from '../lib/dates';
import { SLOTS, closureSchema, toClosureInput, toClosureValues } from './closures.schema';

function todayIso(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** Configurazione → Chiusure: extraordinary closure days, whole day or one slot. */
export function ClosuresPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const thisYear = new Date().getFullYear();
  const [year, setYear] = useState(thisYear);
  const list = closureDays.useList(ctx.establishmentId, {
    from: `${year}-01-01`,
    to: `${year}-12-31`,
  });
  const remove = closureDays.useRemove(ctx.establishmentId);
  const editor = useEditor<ClosureDay>();
  const readOnly = !ctx.canConfigure;
  const items = [...(list.data ?? [])].sort((a, b) => a.date.localeCompare(b.date));
  const years = [thisYear - 1, thisYear, thisYear + 1, thisYear + 2];

  return (
    <ConfigPage
      title={t('closures.title')}
      description={t('closures.description')}
      readOnly={readOnly}
      actions={!readOnly && <Button onClick={editor.create}>{t('closures.add')}</Button>}
    >
      <div className="flex items-center gap-2">
        <label htmlFor="closures-year" className="text-sm font-medium text-slate-800">
          {t('closures.year')}
        </label>
        <Select
          id="closures-year"
          className="w-28"
          value={year}
          onChange={(e) => setYear(Number(e.target.value))}
        >
          {years.map((y) => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </Select>
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('closures.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[30rem] text-sm">
            <caption className="sr-only">{t('closures.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('closures.fields.date')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('closures.fields.slots')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('closures.fields.reason')}
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">{t('actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((c) => (
                <tr key={c.id} className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-medium">
                    {formatDate(c.date, ctx.uiLocale)}
                  </th>
                  <td className="px-4 py-2">
                    <Badge tone={c.slots.length === 2 ? 'amber' : 'blue'}>
                      {c.slots.length === 2
                        ? t('closures.wholeDay')
                        : t(`closures.slot.${c.slots[0] ?? 'AM'}`)}
                    </Badge>
                  </td>
                  <td className="px-4 py-2 text-slate-700">{c.reason}</td>
                  <td className="px-4 py-2 text-right">
                    {!readOnly && (
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => editor.edit(c)}
                          aria-label={t('closures.editAria', {
                            date: formatDate(c.date, ctx.uiLocale),
                          })}
                        >
                          {t('common.actions.edit', { ns: 'translation' })}
                        </Button>
                        <ConfirmDelete
                          label={t('closures.deleteAria', {
                            date: formatDate(c.date, ctx.uiLocale),
                          })}
                          message={t('closures.deleteConfirm', {
                            date: formatDate(c.date, ctx.uiLocale),
                          })}
                          onConfirm={() => remove.mutateAsync({ id: c.id, version: c.version })}
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
        title={editor.item ? t('closures.editTitle') : t('closures.add')}
        description={t('closures.formHint')}
      >
        <ClosureForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          item={editor.item}
          onDone={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

function ClosureForm({
  establishmentId,
  item,
  onDone,
}: {
  establishmentId: string;
  item?: ClosureDay;
  onDone: () => void;
}) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(closureSchema),
    defaultValues: toClosureValues(item, todayIso()),
  });
  const create = closureDays.useCreate(establishmentId);
  const update = closureDays.useUpdate(establishmentId);
  const s = useCrudSubmit({ form, item, create, update, toInput: toClosureInput, onDone });
  const {
    register,
    formState: { errors },
  } = form;
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <Field id="cl-date" label={t('closures.fields.date')} error={errors.date}>
        <Input {...fieldAria('cl-date', errors.date)} type="date" {...register('date')} />
      </Field>
      <CheckboxGroup
        id="cl-slots"
        legend={t('closures.fields.slots')}
        error={errors.slots as never}
      >
        {SLOTS.map((slot) => (
          <Checkbox
            key={slot}
            id={`cl-slot-${slot}`}
            value={slot}
            label={t(`closures.slot.${slot}`)}
            {...register('slots')}
          />
        ))}
      </CheckboxGroup>
      <Field id="cl-reason" label={t('closures.fields.reason')} error={errors.reason}>
        <Input
          {...fieldAria('cl-reason', errors.reason)}
          placeholder={t('closures.reasonPlaceholder')}
          {...register('reason')}
        />
      </Field>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
