import { ApiProblem } from '@arenile/shared/api';
import { Input } from '@arenile/shared/ui/input';
import { FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { FormActions } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { BooleanField, Field, SelectField } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useCreatePrinter, useUpdatePrinter, type Printer } from './api';
import { defaultCharsPerLine } from './lib';
import {
  CODEPAGES,
  CUT_MODES,
  printerSchema,
  toPrinterInput,
  toPrinterValues,
  type PrinterValues,
} from './schemas';

export interface PrinterFormProps {
  establishmentId: string;
  /** Printer being edited; undefined when adding one. */
  item?: Printer;
  onDone: () => void;
}

/**
 * Add / edit an ESC/POS network printer: address (LAN only, port 9100 by
 * default), paper, codepage, cut, feed, logo and cash drawer. A public
 * address is refused here and again by the server (422
 * printer_address_not_allowed, shown on the host field).
 */
export function PrinterForm({ establishmentId, item, onDone }: PrinterFormProps) {
  const { t } = useTranslation('printers');
  const form = useForm<PrinterValues>({
    resolver: zodResolver(printerSchema),
    defaultValues: toPrinterValues(item),
  });
  const create = useCreatePrinter(establishmentId);
  const update = useUpdatePrinter(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const {
    register,
    control,
    setValue,
    getValues,
    formState: { errors },
  } = form;

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    const input = toPrinterInput(values);
    try {
      const saved = item
        ? await update.mutateAsync({ id: item.id, version: item.version, input })
        : await create.mutateAsync(input);
      toast.success(t(item ? 'toast.saved' : 'toast.created', { name: saved.name }));
      onDone();
    } catch (err) {
      if (err instanceof ApiProblem && err.type === 'printer_address_not_allowed') {
        form.setError('host', { type: 'client', message: 'printers:validation.hostPublic' });
        return;
      }
      if (err instanceof ApiProblem && err.status === 409 && err.type === 'conflict') {
        form.setError('name', { type: 'client', message: 'printers:validation.nameTaken' });
        return;
      }
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(getValues()) }));
    }
  });

  // Paper width drives the default characters per line, unless the user changed it.
  const onWidthChange = (width: string) => {
    const previous = width === '58' ? 80 : 58;
    if (getValues('chars_per_line') === String(defaultCharsPerLine(previous))) {
      setValue('chars_per_line', String(defaultCharsPerLine(width === '58' ? 58 : 80)));
    }
  };

  return (
    <form noValidate onSubmit={submit} className="space-y-6">
      <FormAlert state={alert} onReload={onDone} />
      <div className="grid gap-5 sm:grid-cols-2">
        <Field
          id="printer-name"
          label={t('fields.name')}
          error={errors.name}
          className="sm:col-span-2"
        >
          <Input
            {...fieldAria('printer-name', errors.name)}
            autoComplete="off"
            placeholder={t('fields.namePlaceholder')}
            {...register('name')}
          />
        </Field>
        <Field
          id="printer-host"
          label={t('fields.host')}
          error={errors.host}
          hint={t('fields.hostHint')}
        >
          <Input
            {...fieldAria('printer-host', errors.host, true)}
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            inputMode="url"
            placeholder="192.168.1.50"
            {...register('host')}
          />
        </Field>
        <Field
          id="printer-port"
          label={t('fields.port')}
          error={errors.port}
          hint={t('fields.portHint')}
        >
          <Input
            {...fieldAria('printer-port', errors.port, true)}
            inputMode="numeric"
            {...register('port')}
          />
        </Field>
      </div>

      <FieldSet className="gap-5">
        <FieldLegend variant="label">{t('sections.paper')}</FieldLegend>
        <div className="grid gap-5 sm:grid-cols-2">
          <SelectField
            control={control}
            name="paper_width_mm"
            id="printer-width"
            label={t('fields.paperWidth')}
            options={[
              { value: '58', label: t('paper.58') },
              { value: '80', label: t('paper.80') },
            ]}
            onValueChange={onWidthChange}
          />
          <Field
            id="printer-chars"
            label={t('fields.charsPerLine')}
            error={errors.chars_per_line}
            hint={t('fields.charsPerLineHint')}
          >
            <Input
              {...fieldAria('printer-chars', errors.chars_per_line, true)}
              inputMode="numeric"
              {...register('chars_per_line')}
            />
          </Field>
          <SelectField
            control={control}
            name="codepage"
            id="printer-codepage"
            label={t('fields.codepage')}
            hint={t('fields.codepageHint')}
            options={CODEPAGES.map((c) => ({ value: c, label: t(`codepage.${c}`) }))}
          />
          <Field
            id="printer-table"
            label={t('fields.codepageTable')}
            error={errors.codepage_table}
            hint={t('fields.codepageTableHint')}
          >
            <Input
              {...fieldAria('printer-table', errors.codepage_table, true)}
              inputMode="numeric"
              placeholder={t('fields.codepageTableAuto')}
              {...register('codepage_table')}
            />
          </Field>
          <SelectField
            control={control}
            name="cut_mode"
            id="printer-cut"
            label={t('fields.cutMode')}
            options={CUT_MODES.map((c) => ({ value: c, label: t(`cut.${c}`) }))}
          />
          <Field
            id="printer-feed"
            label={t('fields.feedLines')}
            error={errors.feed_lines}
            hint={t('fields.feedLinesHint')}
          >
            <Input
              {...fieldAria('printer-feed', errors.feed_lines, true)}
              inputMode="numeric"
              {...register('feed_lines')}
            />
          </Field>
        </div>
      </FieldSet>

      <FieldSet className="gap-4">
        <FieldLegend variant="label">{t('sections.extras')}</FieldLegend>
        <SelectField
          control={control}
          name="drawer_pin"
          id="printer-drawer"
          label={t('fields.drawer')}
          hint={t('fields.drawerHint')}
          options={[
            { value: 'none', label: t('drawer.none') },
            { value: '2', label: t('drawer.pin2') },
            { value: '5', label: t('drawer.pin5') },
          ]}
        />
        <BooleanField
          control={control}
          name="print_logo"
          id="printer-logo"
          variant="switch"
          label={t('fields.printLogo')}
          description={t('fields.printLogoHint')}
        />
        <BooleanField
          control={control}
          name="active"
          id="printer-active"
          variant="switch"
          label={t('fields.active')}
          description={t('fields.activeHint')}
        />
      </FieldSet>
      <FormActions onCancel={onDone} pending={create.isPending || update.isPending} />
    </form>
  );
}
