import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Calendar } from '@arenile/shared/ui/calendar';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { FieldDescription, FieldLabel, Field as UiField } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { dateFnsLocale, dateToIso, formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FormActions,
  FormDialog,
  QueryState,
  RowActions,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { CalendarOffIcon, PlusIcon } from 'lucide-react';
import { useState } from 'react';
import {
  useController,
  useForm,
  type FieldError as FieldErrorType,
  type Path,
  type UseFormReturn,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { closureDays, type ClosureDay } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { ConfigPage } from '../components/ConfigPage';
import { DateField, Field, FieldError, MultiCheck } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { fieldAria } from '../lib/aria';
import { problemToFormState, type FormAlertState } from '../lib/formErrors';
import {
  SLOTS,
  closureBatchSchema,
  closureSchema,
  toClosureBatchValues,
  toClosureInput,
  toClosureInputs,
  toClosureValues,
} from './closures.schema';

function todayIso(): string {
  return dateToIso(new Date());
}

/** Configurazione → Chiusure: extraordinary closure days, whole day or one slot. */
export function ClosuresPage() {
  const { t, i18n } = useTranslation('configuration');
  const ctx = useConfigContext();
  const thisYear = new Date().getFullYear();
  const [year, setYear] = useState(thisYear);
  const [month, setMonth] = useState<Date>(() => new Date());
  const list = closureDays.useList(ctx.establishmentId, {
    from: `${year}-01-01`,
    to: `${year}-12-31`,
  });
  const remove = closureDays.useRemove(ctx.establishmentId);
  const editor = useEditor<ClosureDay>();
  const [preset, setPreset] = useState<string[]>([]);
  const readOnly = !ctx.canConfigure;
  const items = [...(list.data ?? [])].sort((a, b) => a.date.localeCompare(b.date));
  const years = [thisYear - 1, thisYear, thisYear + 1, thisYear + 2];
  const slotLabel = (c: ClosureDay) =>
    c.slots.length === 2 ? t('closures.wholeDay') : t(`closures.slot.${c.slots[0] ?? 'AM'}`);
  const toDates = (pred: (c: ClosureDay) => boolean) =>
    items.filter(pred).map((c) => isoToDate(c.date) as Date);

  const openNew = (dates: string[] = []) => {
    setPreset(dates);
    editor.create();
  };
  const changeYear = (y: number) => {
    setYear(y);
    setMonth(new Date(y, y === thisYear ? new Date().getMonth() : 5, 1));
  };

  const columns: ColumnDef<ClosureDay>[] = [
    {
      accessorKey: 'date',
      meta: { label: t('closures.fields.date') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('closures.fields.date')} />
      ),
      cell: ({ row }) => (
        <span className="font-medium tabular-nums">{formatIsoDate(row.original.date)}</span>
      ),
    },
    {
      id: 'slots',
      meta: { label: t('closures.fields.slots') },
      header: t('closures.fields.slots'),
      cell: ({ row }) => (
        <StatusBadge tone={row.original.slots.length === 2 ? 'danger' : 'warning'}>
          {slotLabel(row.original)}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'reason',
      meta: { label: t('closures.fields.reason') },
      header: t('closures.fields.reason'),
      cell: ({ row }) => <span className="text-muted-foreground">{row.original.reason}</span>,
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const date = formatIsoDate(row.original.date);
        return readOnly ? null : (
          <RowActions
            editLabel={t('closures.editAria', { date })}
            deleteLabel={t('closures.deleteAria', { date })}
            deleteMessage={t('closures.deleteConfirm', { date })}
            onEdit={() => editor.edit(row.original)}
            onDelete={() =>
              remove.mutateAsync({ id: row.original.id, version: row.original.version })
            }
          />
        );
      },
    },
  ];

  const addButton = !readOnly && (
    <Button onClick={() => openNew()}>
      <PlusIcon aria-hidden="true" />
      {t('closures.add')}
    </Button>
  );

  return (
    <ConfigPage
      title={t('closures.title')}
      description={t('closures.description')}
      readOnly={readOnly}
      actions={addButton}
    >
      <div className="flex items-center gap-3">
        <Label htmlFor="closures-year">{t('closures.year')}</Label>
        <Select value={String(year)} onValueChange={(v) => changeYear(Number(v))}>
          <SelectTrigger id="closures-year" className="w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {years.map((y) => (
              <SelectItem key={y} value={String(y)}>
                {y}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <div className="grid items-start gap-6 xl:grid-cols-[auto_1fr]">
          <Card className="gap-3">
            <CardHeader>
              <CardTitle>{t('closures.calendarTitle', { year })}</CardTitle>
              <CardDescription>
                {readOnly ? t('closures.calendarHintReadOnly') : t('closures.calendarHint')}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col items-center gap-3">
              <Calendar
                month={month}
                onMonthChange={setMonth}
                startMonth={new Date(year, 0, 1)}
                endMonth={new Date(year, 11, 1)}
                numberOfMonths={2}
                locale={dateFnsLocale(i18n.language)}
                weekStartsOn={1}
                modifiers={{
                  closedDay: toDates((c) => c.slots.length === 2),
                  closedSlot: toDates((c) => c.slots.length === 1),
                }}
                modifiersClassNames={{
                  closedDay:
                    '[&>button]:bg-destructive/15 [&>button]:text-destructive [&>button]:font-semibold',
                  closedSlot: '[&>button]:bg-warning/25 [&>button]:font-semibold',
                }}
                onDayClick={(day) => {
                  if (readOnly) return;
                  const iso = dateToIso(day);
                  const existing = items.find((c) => c.date === iso);
                  if (existing) editor.edit(existing);
                  else openNew([iso]);
                }}
              />
              <ul className="flex flex-wrap gap-2 text-xs" aria-label={t('closures.legend')}>
                <li>
                  <Badge variant="secondary" className="bg-destructive/15 text-destructive">
                    {t('closures.wholeDay')}
                  </Badge>
                </li>
                <li>
                  <Badge variant="secondary" className="bg-warning/25 text-foreground">
                    {t('closures.oneSlot')}
                  </Badge>
                </li>
              </ul>
            </CardContent>
          </Card>
          <DataTable
            caption={t('closures.title')}
            columns={columns}
            data={items}
            loading={list.isPending}
            getRowId={(c) => c.id}
            searchable
            pageSize={10}
            columnToggle={false}
            empty={
              <EmptyState icon={CalendarOffIcon} title={t('closures.empty')} bordered={false} />
            }
          />
        </div>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('closures.editTitle') : t('closures.add')}
        description={t('closures.formHint')}
      >
        {editor.item ? (
          <ClosureForm
            key={editor.key}
            establishmentId={ctx.establishmentId}
            item={editor.item}
            onDone={editor.close}
          />
        ) : (
          <NewClosuresForm
            key={editor.key}
            establishmentId={ctx.establishmentId}
            year={year}
            initialDates={preset}
            taken={items.map((c) => c.date)}
            onDone={editor.close}
          />
        )}
      </FormDialog>
    </ConfigPage>
  );
}

function SlotsAndReason<T extends { slots: string[]; reason: string }>({
  form,
}: {
  form: UseFormReturn<T>;
}) {
  const { t } = useTranslation('configuration');
  const reasonError = form.formState.errors.reason as FieldErrorType | undefined;
  return (
    <>
      <MultiCheck
        control={form.control}
        name={'slots' as Path<T>}
        id="cl-slots"
        legend={t('closures.fields.slots')}
        options={SLOTS.map((slot) => ({ value: slot, label: t(`closures.slot.${slot}`) }))}
      />
      <Field id="cl-reason" label={t('closures.fields.reason')} error={reasonError}>
        <Input
          {...fieldAria('cl-reason', reasonError)}
          placeholder={t('closures.reasonPlaceholder')}
          {...form.register('reason' as Path<T>)}
        />
      </Field>
    </>
  );
}

/** Edit one closure day. */
function ClosureForm({
  establishmentId,
  item,
  onDone,
}: {
  establishmentId: string;
  item: ClosureDay;
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
  return (
    <form noValidate onSubmit={s.submit} className="space-y-5">
      <FormAlert state={s.alert} onReload={onDone} />
      <DateField
        control={form.control}
        name="date"
        id="cl-date"
        label={t('closures.fields.date')}
      />
      <SlotsAndReason form={form} />
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}

/** Create closures on several days at once (shadcn Calendar, multiple selection). */
function NewClosuresForm({
  establishmentId,
  year,
  initialDates,
  taken,
  onDone,
}: {
  establishmentId: string;
  year: number;
  initialDates: string[];
  taken: string[];
  onDone: () => void;
}) {
  const { t, i18n } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(closureBatchSchema),
    defaultValues: toClosureBatchValues(initialDates),
  });
  const create = closureDays.useCreate(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const dates = useController({ control: form.control, name: 'dates' });
  const selected = (dates.field.value ?? []).map((d) => isoToDate(d) as Date);
  const first = isoToDate(initialDates[0]) ?? new Date(year, new Date().getMonth(), 1);

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    let done = 0;
    try {
      for (const input of toClosureInputs(values)) {
        await create.mutateAsync(input);
        done += 1;
      }
      toast.success(t('closures.createdMany', { count: done }));
      onDone();
    } catch (err) {
      if (done > 0) toast.success(t('closures.createdMany', { count: done }));
      setAlert(
        problemToFormState(err, form.setError, {
          formKeys: ['slots', 'reason'],
          aliases: { date: 'dates' },
        }),
      );
    }
  });

  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      <FormAlert state={alert} onReload={onDone} />
      <UiField className="gap-2">
        <FieldLabel id="cl-dates-label">{t('closures.fields.dates')}</FieldLabel>
        <div className="flex justify-center rounded-lg border">
          <Calendar
            mode="multiple"
            aria-labelledby="cl-dates-label"
            selected={selected}
            onSelect={(days) => dates.field.onChange((days ?? []).map(dateToIso).sort())}
            defaultMonth={first}
            locale={dateFnsLocale(i18n.language)}
            weekStartsOn={1}
            disabled={taken.map((d) => isoToDate(d) as Date)}
          />
        </div>
        <FieldDescription aria-live="polite">
          {selected.length > 0
            ? t('closures.selectedDays', {
                count: selected.length,
                list: (dates.field.value ?? []).map(formatIsoDate).join(', '),
              })
            : t('closures.pickDays')}
        </FieldDescription>
        <FieldError id="cl-dates" error={dates.fieldState.error} />
      </UiField>
      <SlotsAndReason form={form} />
      <FormActions
        onCancel={onDone}
        pending={create.isPending}
        submitLabel={
          selected.length > 1 ? t('closures.createMany', { count: selected.length }) : undefined
        }
      />
    </form>
  );
}
