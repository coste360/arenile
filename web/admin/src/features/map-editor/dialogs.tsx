import { SPOT_KIND_ICONS } from '@arenile/shared/map';
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Checkbox,
  FieldLegend,
  FieldSet,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
} from '@arenile/shared/ui';
import { FieldRow, I18nTextInput } from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { CircleAlertIcon, EyeIcon, PlusIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useForm, useWatch, type Control, type FieldPath, type FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useRowPreview } from './api';
import {
  generatorDefaults,
  generatorSchema,
  mapSettingsDefaults,
  mapSettingsSchema,
  renumberDefaults,
  renumberSchema,
  toGeneratorRequest,
  toMapSettings,
  type GeneratorValues,
  type MapSettingsValues,
  type RenumberValues,
} from './editor.schema';
import { spotFromInput } from './model/document';
import { conflictingLabels, formatSpotLabel, type RenumberOptions } from './model/editor';
import {
  SEA_SIDES,
  SPOT_KINDS,
  type DraftDocument,
  type DraftMapSettings,
  type DraftSpot,
  type RowGeneratorResult,
} from './model/types';
import { FormActions, FormAlert } from './ui';

export interface ZoneOption {
  id: string;
  name: string;
  disabled?: boolean;
}

interface Choice {
  value: string;
  label: ReactNode;
  disabled?: boolean;
}

/** A shadcn Select bound to a form field (label, trigger, validation message). */
function SelectRow<T extends FieldValues>({
  control,
  name,
  label,
  options,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  options: Choice[];
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <Select value={field.value as string} onValueChange={field.onChange}>
            <FormControl>
              <SelectTrigger className="w-full" onBlur={field.onBlur}>
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {options.map((o) => (
                <SelectItem key={o.value} value={o.value} disabled={o.disabled}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

/**
 * Row generator (spec 11.2): parameters → server preview (nothing written)
 * → the spots are added to the draft and saved with the map.
 */
export function RowGeneratorForm({
  establishmentId,
  mapId,
  doc,
  zones,
  onAdd,
  onCancel,
}: {
  establishmentId: string;
  mapId: string;
  doc: DraftDocument;
  zones: ZoneOption[];
  onAdd: (spots: DraftSpot[]) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  const firstZone = zones.find((z) => !z.disabled)?.id ?? '';
  const form = useForm<GeneratorValues>({
    resolver: zodResolver(generatorSchema),
    defaultValues: generatorDefaults(doc.map, firstZone),
  });
  const preview = useRowPreview(establishmentId, mapId);
  const [result, setResult] = useState<RowGeneratorResult | null>(null);
  const [alert, setAlert] = useState<FormAlertState | null>(null);

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      setResult(await preview.mutateAsync(toGeneratorRequest(values)));
    } catch (err) {
      setResult(null);
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });

  // Labels clashing with the server map or with unsaved spots of the draft.
  const conflicts = result
    ? [...new Set([...result.conflicting_labels, ...conflictingLabels(doc, result.spots)])]
    : [];

  const text = (name: keyof GeneratorValues, label: string, numeric = true) => (
    <FieldRow control={form.control} name={name} label={label}>
      {(field) => (
        <Input
          inputMode={numeric ? 'decimal' : undefined}
          {...field}
          value={field.value as string}
        />
      )}
    </FieldRow>
  );

  return (
    <Form {...form}>
      <form noValidate onSubmit={submit} className="space-y-6">
        <FormAlert state={alert} />
        <FieldSet>
          <FieldLegend>{t('generator.layout')}</FieldLegend>
          <div className="grid gap-4 sm:grid-cols-3">
            <SelectRow
              control={form.control}
              name="zone_id"
              label={t('generator.zone')}
              options={zones.map((z) => ({ value: z.id, label: z.name, disabled: z.disabled }))}
            />
            <SelectRow
              control={form.control}
              name="kind"
              label={t('fields.kind')}
              options={SPOT_KINDS.map((k) => {
                const Icon = SPOT_KIND_ICONS[k];
                return {
                  value: k,
                  label: (
                    <>
                      <Icon aria-hidden="true" />
                      {t(`kinds.${k}`)}
                    </>
                  ),
                };
              })}
            />
            <div className="hidden sm:block" />
            {text('rows', t('generator.rows'))}
            {text('spots_per_row', t('generator.spotsPerRow'))}
            <div className="hidden sm:block" />
            {text('origin_x', t('generator.originX'))}
            {text('origin_y', t('generator.originY'))}
            <div className="hidden sm:block" />
            {text('spacing_x', t('generator.spacingX'))}
            {text('spacing_y', t('generator.spacingY'))}
          </div>
          <p className="text-sm text-muted-foreground">{t('generator.seaHint')}</p>
        </FieldSet>
        <Separator />
        <FieldSet>
          <FieldLegend>{t('generator.labels')}</FieldLegend>
          <div className="grid gap-4 sm:grid-cols-3">
            <SelectRow
              control={form.control}
              name="row_label_style"
              label={t('generator.rowLabels')}
              options={[
                { value: 'letters', label: t('generator.letters') },
                { value: 'numbers', label: t('generator.numbers') },
              ]}
            />
            {text('first_row_label', t('generator.firstRow'), false)}
            {text('number_start', t('generator.numberStart'))}
            <SelectRow
              control={form.control}
              name="direction"
              label={t('generator.direction')}
              options={[
                { value: 'ltr', label: t('generator.ltr') },
                { value: 'rtl', label: t('generator.rtl') },
              ]}
            />
            <SelectRow
              control={form.control}
              name="numbering"
              label={t('generator.numbering')}
              options={[
                { value: 'per_row', label: t('generator.perRow') },
                { value: 'continuous', label: t('generator.continuous') },
              ]}
            />
            {text('label_format', t('generator.labelFormat'), false)}
          </div>
        </FieldSet>
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="outline" onClick={onCancel}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </Button>
          <Button type="submit" variant="secondary" disabled={preview.isPending}>
            {preview.isPending ? <Spinner /> : <EyeIcon aria-hidden="true" />}
            {t('generator.preview')}
          </Button>
        </div>
        {result && (
          <Alert role="status" variant={conflicts.length > 0 ? 'destructive' : 'default'}>
            {conflicts.length > 0 ? (
              <CircleAlertIcon aria-hidden="true" />
            ) : (
              <EyeIcon aria-hidden="true" />
            )}
            <AlertTitle>{t('generator.previewTitle')}</AlertTitle>
            <AlertDescription className="space-y-3">
              <p>
                {t('generator.previewCount', {
                  count: result.spots.length,
                  first: result.spots[0]?.label ?? '',
                  last: result.spots[result.spots.length - 1]?.label ?? '',
                })}
              </p>
              {conflicts.length > 0 && (
                <p>{t('generator.conflicts', { labels: conflicts.slice(0, 10).join(', ') })}</p>
              )}
              <div className="flex w-full flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap gap-1" aria-hidden="true">
                  {result.spots.slice(0, 8).map((s) => (
                    <Badge key={s.label} variant="secondary" className="font-mono">
                      {s.label}
                    </Badge>
                  ))}
                  {result.spots.length > 8 && <Badge variant="outline">…</Badge>}
                </div>
                <Button
                  disabled={conflicts.length > 0 || result.spots.length === 0}
                  onClick={() => onAdd(result.spots.map(spotFromInput))}
                >
                  <PlusIcon aria-hidden="true" />
                  {t('generator.add', { count: result.spots.length })}
                </Button>
              </div>
            </AlertDescription>
          </Alert>
        )}
      </form>
    </Form>
  );
}

/** Name, size, grid and sea side of a map (create on the list page, edit in the editor). */
export function MapSettingsForm({
  initial,
  locales,
  defaultLocale,
  pending,
  alert,
  onSubmit,
  onCancel,
  submitLabel,
}: {
  initial?: Pick<DraftMapSettings, 'name' | 'width' | 'height' | 'grid_size' | 'sea_side'>;
  locales: string[];
  defaultLocale: string;
  pending?: boolean;
  alert?: FormAlertState | null;
  onSubmit: (values: ReturnType<typeof toMapSettings>) => void | Promise<void>;
  onCancel: () => void;
  submitLabel?: string;
}) {
  const { t } = useTranslation('mapEditor');
  const form = useForm<MapSettingsValues>({
    resolver: zodResolver(mapSettingsSchema(defaultLocale)),
    defaultValues: mapSettingsDefaults(initial, locales),
  });
  const nameErrors = form.formState.errors.name as
    (Record<string, { message?: string }> & { message?: string }) | undefined;
  const nameError = nameErrors?.message ?? nameErrors?.[defaultLocale]?.message;
  return (
    <Form {...form}>
      <form
        noValidate
        onSubmit={form.handleSubmit((v) => onSubmit(toMapSettings(v)))}
        className="space-y-5"
      >
        <FormAlert state={alert ?? null} />
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('fields.name')}</FormLabel>
              <FormControl>
                <I18nTextInput
                  value={field.value}
                  onChange={field.onChange}
                  locales={locales}
                  defaultLocale={defaultLocale}
                />
              </FormControl>
              {nameError && (
                <p className="text-sm text-destructive">{t(nameError, { ns: 'translation' })}</p>
              )}
            </FormItem>
          )}
        />
        <div className="grid gap-4 sm:grid-cols-4">
          {(['width', 'height', 'grid_size'] as const).map((name) => (
            <FieldRow key={name} control={form.control} name={name} label={t(`fields.${name}`)}>
              {(field) => <Input inputMode="numeric" {...field} />}
            </FieldRow>
          ))}
          <SelectRow
            control={form.control}
            name="sea_side"
            label={t('fields.sea_side')}
            options={SEA_SIDES.map((s) => ({ value: s, label: t(`seaSides.${s}`) }))}
          />
        </div>
        <FormActions onCancel={onCancel} pending={!!pending} submitLabel={submitLabel} />
      </form>
    </Form>
  );
}

/** Renumbers the selected spots as one row, ordered along the shoreline. */
export function RenumberRowForm({
  spots,
  onSubmit,
  onCancel,
}: {
  spots: readonly DraftSpot[];
  onSubmit: (opts: RenumberOptions) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  const form = useForm<RenumberValues>({
    resolver: zodResolver(renumberSchema),
    defaultValues: renumberDefaults(spots),
  });
  const [row, start, format] = useWatch({
    control: form.control,
    name: ['row_label', 'start', 'format'],
  });
  const first = Number(start);
  const example = Number.isInteger(first)
    ? `${formatSpotLabel(format, row.trim(), first)} … ${formatSpotLabel(
        format,
        row.trim(),
        first + spots.length - 1,
      )}`
    : '';
  return (
    <Form {...form}>
      <form
        noValidate
        className="space-y-5"
        onSubmit={form.handleSubmit((v) =>
          onSubmit({
            rowLabel: v.row_label,
            start: Number(v.start),
            reverse: v.reverse,
            format: v.format || undefined,
          }),
        )}
      >
        <div className="grid gap-4 sm:grid-cols-3">
          <FieldRow control={form.control} name="row_label" label={t('fields.row_label')}>
            {(field) => <Input {...field} />}
          </FieldRow>
          <FieldRow control={form.control} name="start" label={t('generator.numberStart')}>
            {(field) => <Input inputMode="numeric" {...field} />}
          </FieldRow>
          <FieldRow control={form.control} name="format" label={t('generator.labelFormat')}>
            {(field) => <Input {...field} />}
          </FieldRow>
        </div>
        <FormField
          control={form.control}
          name="reverse"
          render={({ field }) => (
            <FormItem className="flex items-center gap-3">
              <FormControl>
                <Checkbox
                  checked={field.value}
                  onCheckedChange={(v) => field.onChange(v === true)}
                />
              </FormControl>
              <FormLabel className="font-normal">{t('renumber.reverse')}</FormLabel>
            </FormItem>
          )}
        />
        <p
          className="rounded-md bg-muted px-3 py-2 font-mono text-sm text-muted-foreground"
          aria-live="polite"
        >
          {t('renumber.example', { count: spots.length, example })}
        </p>
        <FormActions onCancel={onCancel} submitLabel={t('renumber.apply')} />
      </form>
    </Form>
  );
}
