import { Button } from '@arenile/shared/ui/button';
import { EmptyState, SectionCard } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { zodResolver } from '@hookform/resolvers/zod';
import { PlusIcon, RouteIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import { useFieldArray, useForm, useWatch, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { z } from 'zod';
import { BooleanField, SelectField } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  useUpdatePrintRoutes,
  type CashRegister,
  type Printer,
  type VersionedRouteSet,
} from './api';
import { ROUTABLE_KINDS, compareRows, toRoutes, toRows, type RouteRow } from './lib';

const rowSchema = z.object({
  kind: z.enum(ROUTABLE_KINDS),
  registerId: z.string(),
  printerId: z.string().min(1, 'printers:validation.printerRequired'),
  copies: z.enum(['1', '2', '3']),
  kickDrawer: z.boolean(),
});

const routesSchema = z.object({
  rows: z.array(rowSchema).superRefine((rows, ctx) => {
    const seen = new Set<string>();
    rows.forEach((r, i) => {
      const key = `${r.kind}|${r.registerId}`;
      if (seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          path: [i, 'registerId'],
          message: 'printers:validation.duplicateRoute',
        });
      }
      seen.add(key);
    });
  }),
});

type RoutesValues = z.infer<typeof routesSchema>;
type RowValues = RoutesValues['rows'][number];

function toValues(set: VersionedRouteSet): RoutesValues {
  return {
    rows: toRows(set.items).map((r) => ({ ...r, copies: String(r.copies) as RowValues['copies'] })),
  };
}

function fromValues(values: RoutesValues): RouteRow[] {
  return values.rows.map((r) => ({ ...r, copies: Number(r.copies) })).sort(compareRows);
}

export interface RoutingCardProps {
  establishmentId: string;
  routes: VersionedRouteSet;
  printers: Printer[];
  registers: CashRegister[];
  readOnly: boolean;
  /** Reloads the set after a 412 version_conflict. */
  onReload: () => void;
}

/**
 * Instradamento: which printer prints each document, with an optional rule
 * per cash register that wins over the default one. Without a rule the
 * document is printed from the browser. The whole set is saved at once
 * (PUT with If-Match on the ETag of the set).
 */
export function RoutingCard({
  establishmentId,
  routes,
  printers,
  registers,
  readOnly,
  onReload,
}: RoutingCardProps) {
  const { t } = useTranslation('printers');
  const form = useForm<RoutesValues>({
    resolver: zodResolver(routesSchema),
    defaultValues: toValues(routes),
  });
  const rows = useFieldArray({ control: form.control, name: 'rows' });
  const save = useUpdatePrintRoutes(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const saved = await save.mutateAsync({
        items: toRoutes(fromValues(values)),
        etag: routes.etag,
      });
      form.reset(toValues(saved));
      toast.success(t('routing.saved'));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: ['rows'] }));
    }
  });

  const firstPrinter = printers.find((p) => p.active)?.id ?? '';
  const addRow = () => {
    const used = new Set(form.getValues('rows').map((r) => `${r.kind}|${r.registerId}`));
    const kind = ROUTABLE_KINDS.find((k) => !used.has(`${k}|`)) ?? ROUTABLE_KINDS[0];
    const registerId = used.has(`${kind}|`)
      ? (registers.find((r) => !used.has(`${kind}|${r.id}`))?.id ?? '')
      : '';
    rows.append({ kind, registerId, printerId: firstPrinter, copies: '1', kickDrawer: false });
  };

  const printerOptions = printers.map((p) => ({
    value: p.id,
    label: p.active ? p.name : t('routing.inactivePrinter', { name: p.name }),
  }));
  const registerOptions = [
    { value: '', label: t('routing.allRegisters') },
    ...registers.map((r) => ({ value: r.id, label: r.name })),
  ];

  return (
    <SectionCard
      title={t('routing.title')}
      titleId="printers-routing-title"
      icon={RouteIcon}
      description={t('routing.description')}
      actions={
        !readOnly &&
        printers.length > 0 && (
          <Button variant="outline" size="sm" onClick={addRow}>
            <PlusIcon aria-hidden="true" />
            {t('routing.add')}
          </Button>
        )
      }
    >
      <form
        noValidate
        onSubmit={submit}
        aria-labelledby="printers-routing-title"
        className="space-y-4"
      >
        <FormAlert state={alert} onReload={onReload} />
        {rows.fields.length === 0 ? (
          <EmptyState
            icon={RouteIcon}
            title={t('routing.empty')}
            description={
              printers.length === 0 ? t('routing.emptyNoPrinter') : t('routing.emptyHint')
            }
            bordered={false}
          />
        ) : (
          <ul className="space-y-3">
            {rows.fields.map((f, i) => (
              <li key={f.id}>
                <RouteRowFields
                  control={form.control}
                  index={i}
                  printers={printers}
                  printerOptions={printerOptions}
                  registerOptions={registerOptions}
                  readOnly={readOnly}
                  onRemove={() => rows.remove(i)}
                />
              </li>
            ))}
          </ul>
        )}
        {!readOnly && (rows.fields.length > 0 || form.formState.isDirty) && (
          <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
            <Button
              variant="outline"
              disabled={!form.formState.isDirty || save.isPending}
              onClick={() => {
                setAlert(null);
                form.reset(toValues(routes));
              }}
            >
              {t('routing.discard')}
            </Button>
            <Button type="submit" disabled={!form.formState.isDirty || save.isPending}>
              {save.isPending && <Spinner />}
              {t('routing.save')}
            </Button>
          </div>
        )}
      </form>
    </SectionCard>
  );
}

function RouteRowFields({
  control,
  index,
  printers,
  printerOptions,
  registerOptions,
  readOnly,
  onRemove,
}: {
  control: Control<RoutesValues>;
  index: number;
  printers: Printer[];
  printerOptions: { value: string; label: string }[];
  registerOptions: { value: string; label: string }[];
  readOnly: boolean;
  onRemove: () => void;
}) {
  const { t } = useTranslation('printers');
  const printerId = useWatch({ control, name: `rows.${index}.printerId` });
  const kind = useWatch({ control, name: `rows.${index}.kind` });
  const hasDrawer = printers.find((p) => p.id === printerId)?.drawer_pin !== undefined;
  const id = `route-${index}`;
  return (
    <fieldset
      aria-label={t('routing.rowAria', { index: index + 1, document: t(`kind.${kind}`) })}
      className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-12 lg:items-end"
    >
      <SelectField
        control={control}
        name={`rows.${index}.kind`}
        id={`${id}-kind`}
        label={t('routing.document')}
        disabled={readOnly}
        className="lg:col-span-3"
        options={ROUTABLE_KINDS.map((k) => ({ value: k, label: t(`kind.${k}`) }))}
      />
      <SelectField
        control={control}
        name={`rows.${index}.registerId`}
        id={`${id}-register`}
        label={t('routing.register')}
        disabled={readOnly}
        className="lg:col-span-3"
        options={registerOptions}
      />
      <SelectField
        control={control}
        name={`rows.${index}.printerId`}
        id={`${id}-printer`}
        label={t('routing.printer')}
        placeholder={t('routing.choosePrinter')}
        disabled={readOnly}
        className="lg:col-span-3"
        options={printerOptions}
      />
      <SelectField
        control={control}
        name={`rows.${index}.copies`}
        id={`${id}-copies`}
        label={t('routing.copies')}
        disabled={readOnly}
        className="lg:col-span-1"
        options={['1', '2', '3'].map((c) => ({ value: c, label: c }))}
      />
      <div className="flex items-center justify-between gap-2 sm:col-span-2 lg:col-span-2 lg:justify-end">
        <BooleanField
          control={control}
          name={`rows.${index}.kickDrawer`}
          id={`${id}-drawer`}
          label={t('routing.kickDrawer')}
          disabled={readOnly || !hasDrawer}
          className="w-auto"
        />
        {!readOnly && (
          <Button
            variant="ghost"
            size="icon"
            onClick={onRemove}
            aria-label={t('routing.remove', { index: index + 1 })}
          >
            <XIcon aria-hidden="true" />
          </Button>
        )}
      </div>
    </fieldset>
  );
}
