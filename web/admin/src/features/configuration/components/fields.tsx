import { Checkbox } from '@arenile/shared/ui/checkbox';
import {
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
  Field as UiField,
  FieldError as UiFieldError,
} from '@arenile/shared/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from '@arenile/shared/ui/input-group';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { Switch } from '@arenile/shared/ui/switch';
import { cn } from '@arenile/shared/lib/utils';
import { DatePicker, DateRangePicker, I18nTextInput } from '@arenile/shared/ui/patterns';
import type { ComponentProps, ReactNode } from 'react';
import {
  useController,
  useFormState,
  type Control,
  type FieldError as RhfFieldError,
  type FieldValues,
  type Path,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { errorAt, fieldAria } from '../lib/aria';

/*
 * Adapters between react-hook-form and the shadcn/ui field components
 * (Field, Select, Checkbox, Switch, InputGroup) and the shared patterns
 * (DatePicker, DateRangePicker, I18nTextInput). Client errors carry an i18n
 * key; errors mapped from problem+json (`type: "server"`) carry the server
 * text verbatim and are never passed through t().
 */

function useErrorText() {
  const { t } = useTranslation();
  return (error: RhfFieldError | undefined) =>
    error?.message ? (error.type === 'server' ? error.message : t(error.message)) : '';
}

/** Error under an input; its id is `${id}-error` (see fieldAria). */
export function FieldError({ id, error }: { id: string; error: RhfFieldError | undefined }) {
  const text = useErrorText()(error);
  if (!text) return null;
  return <UiFieldError id={`${id}-error`}>{text}</UiFieldError>;
}

export interface FieldProps {
  id: string;
  label: ReactNode;
  error?: RhfFieldError;
  hint?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** shadcn Field: label + control + description + error. The control uses `fieldAria(id, error, !!hint)`. */
export function Field({ id, label, error, hint, className, children }: FieldProps) {
  return (
    <UiField data-invalid={error ? true : undefined} className={cn('gap-2', className)}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {children}
      {hint && <FieldDescription id={`${id}-hint`}>{hint}</FieldDescription>}
      <FieldError id={id} error={error} />
    </UiField>
  );
}

/** Input with a trailing unit ("%", "€", "giorni") on shadcn InputGroup; works with `register`. */
export function SuffixInput({
  suffix,
  className,
  ...props
}: ComponentProps<'input'> & { suffix: string }) {
  return (
    <InputGroup className={className} data-disabled={props.disabled || undefined}>
      <InputGroupInput {...props} className="tabular-nums" />
      <InputGroupAddon align="inline-end">
        <InputGroupText aria-hidden="true">{suffix}</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  );
}

// ---- Select ---------------------------------------------------------------------------

/** Radix Select item values cannot be empty: "" is mapped to this sentinel. */
const NONE = '__none__';

export interface SelectOption {
  value: string;
  label: ReactNode;
  icon?: ReactNode;
}

export interface SelectFieldProps<T extends FieldValues> {
  control: Control<T>;
  name: Path<T>;
  id: string;
  label: ReactNode;
  options: SelectOption[];
  hint?: ReactNode;
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  onValueChange?: (value: string) => void;
}

/** shadcn Select bound to a string field, with label, hint and error. */
export function SelectField<T extends FieldValues>({
  control,
  name,
  id,
  label,
  options,
  hint,
  disabled,
  placeholder,
  className,
  onValueChange,
}: SelectFieldProps<T>) {
  const {
    field: { ref, ...field },
    fieldState,
  } = useController({ control, name });
  const value = String(field.value ?? '');
  return (
    <Field id={id} label={label} error={fieldState.error} hint={hint} className={className}>
      <Select
        value={value === '' ? NONE : value}
        onValueChange={(v) => {
          const next = v === NONE ? '' : v;
          field.onChange(next);
          onValueChange?.(next);
        }}
        disabled={disabled}
        name={field.name}
      >
        <SelectTrigger
          ref={ref}
          onBlur={field.onBlur}
          className="w-full"
          {...fieldAria(id, fieldState.error, !!hint)}
        >
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value || NONE} value={o.value === '' ? NONE : o.value}>
              {o.icon}
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

// ---- Boolean -------------------------------------------------------------------------

export interface BooleanFieldProps<T extends FieldValues> {
  control: Control<T>;
  name: Path<T>;
  id: string;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
  /** "switch" for settings that turn a feature on/off (default "checkbox"). */
  variant?: 'checkbox' | 'switch';
  className?: string;
}

/** shadcn Checkbox or Switch bound to a boolean field, label on the right. */
export function BooleanField<T extends FieldValues>({
  control,
  name,
  id,
  label,
  description,
  disabled,
  variant = 'checkbox',
  className,
}: BooleanFieldProps<T>) {
  const { field } = useController({ control, name });
  const common = {
    id,
    ref: field.ref,
    checked: !!field.value,
    disabled,
    onBlur: field.onBlur,
    'aria-describedby': description ? `${id}-hint` : undefined,
  };
  return (
    <UiField
      orientation="horizontal"
      data-disabled={disabled || undefined}
      className={cn(variant === 'switch' && 'rounded-lg border p-3', className)}
    >
      {variant === 'switch' ? (
        <>
          <FieldContent>
            <FieldLabel htmlFor={id}>{label}</FieldLabel>
            {description && <FieldDescription id={`${id}-hint`}>{description}</FieldDescription>}
          </FieldContent>
          <Switch {...common} onCheckedChange={(v) => field.onChange(v)} />
        </>
      ) : (
        <>
          <Checkbox {...common} onCheckedChange={(v) => field.onChange(v === true)} />
          <FieldContent>
            <FieldLabel htmlFor={id}>{label}</FieldLabel>
            {description && <FieldDescription id={`${id}-hint`}>{description}</FieldDescription>}
          </FieldContent>
        </>
      )}
    </UiField>
  );
}

// ---- Multiple choice -------------------------------------------------------------------

export interface MultiCheckProps<T extends FieldValues> {
  control: Control<T>;
  name: Path<T>;
  id: string;
  legend: ReactNode;
  options: SelectOption[];
  disabled?: boolean;
  hint?: ReactNode;
}

/**
 * Group of shadcn Checkboxes bound to a string[] field (always an array, in
 * the options' order, independent of click order).
 */
export function MultiCheck<T extends FieldValues>({
  control,
  name,
  id,
  legend,
  options,
  disabled,
  hint,
}: MultiCheckProps<T>) {
  const { field, fieldState } = useController({ control, name });
  const value: string[] = Array.isArray(field.value) ? (field.value as string[]) : [];
  const toggle = (v: string, checked: boolean) => {
    const next = checked ? [...value, v] : value.filter((x) => x !== v);
    field.onChange(options.map((o) => o.value).filter((o) => next.includes(o)));
  };
  return (
    <FieldSet
      className="gap-3"
      aria-describedby={
        [fieldState.error ? `${id}-error` : '', hint ? `${id}-hint` : ''].join(' ').trim() ||
        undefined
      }
    >
      <FieldLegend variant="label">{legend}</FieldLegend>
      <div className="flex flex-wrap gap-x-6 gap-y-3">
        {options.map((o, i) => (
          <UiField
            key={o.value}
            orientation="horizontal"
            className="w-auto"
            data-disabled={disabled || undefined}
          >
            <Checkbox
              id={`${id}-${o.value}`}
              ref={i === 0 ? field.ref : undefined}
              checked={value.includes(o.value)}
              disabled={disabled}
              onCheckedChange={(c) => toggle(o.value, c === true)}
              onBlur={field.onBlur}
            />
            <FieldLabel htmlFor={`${id}-${o.value}`} className="font-normal">
              {o.icon}
              {o.label}
            </FieldLabel>
          </UiField>
        ))}
      </div>
      {hint && <FieldDescription id={`${id}-hint`}>{hint}</FieldDescription>}
      <FieldError id={id} error={fieldState.error} />
    </FieldSet>
  );
}

// ---- Dates -------------------------------------------------------------------------------

export interface DateFieldProps<T extends FieldValues> {
  control: Control<T>;
  name: Path<T>;
  id: string;
  label: ReactNode;
  hint?: ReactNode;
  disabled?: boolean;
  min?: string;
  max?: string;
  className?: string;
}

/** Shared DatePicker (dd/MM/yyyy, never a native date input) bound to a "YYYY-MM-DD" string. */
export function DateField<T extends FieldValues>({
  control,
  name,
  id,
  label,
  hint,
  disabled,
  min,
  max,
  className,
}: DateFieldProps<T>) {
  const { field, fieldState } = useController({ control, name });
  return (
    <Field id={id} label={label} error={fieldState.error} hint={hint} className={className}>
      <DatePicker
        {...fieldAria(id, fieldState.error, !!hint)}
        value={(field.value as string) || undefined}
        onChange={(v) => field.onChange(v ?? '')}
        disabled={disabled}
        min={min}
        max={max}
        className="[&>button]:w-full"
      />
    </Field>
  );
}

export interface DateRangeFieldProps<T extends FieldValues> {
  control: Control<T>;
  /** Paths of the two "YYYY-MM-DD" fields. */
  fromName: Path<T>;
  toName: Path<T>;
  id: string;
  label: string;
  disabled?: boolean;
  min?: string;
  max?: string;
}

/** Shared DateRangePicker bound to two string fields (from/to), errors of both underneath. */
export function DateRangeField<T extends FieldValues>({
  control,
  fromName,
  toName,
  id,
  label,
  disabled,
  min,
  max,
}: DateRangeFieldProps<T>) {
  const from = useController({ control, name: fromName });
  const to = useController({ control, name: toName });
  const error = from.fieldState.error ?? to.fieldState.error;
  return (
    <div className="space-y-1">
      <DateRangePicker
        {...fieldAria(id, error)}
        aria-label={label}
        value={
          from.field.value
            ? { from: from.field.value as string, to: to.field.value as string }
            : undefined
        }
        onChange={(r) => {
          from.field.onChange(r?.from ?? '');
          to.field.onChange(r?.to ?? r?.from ?? '');
        }}
        disabled={disabled}
        min={min}
        max={max}
      />
      <FieldError id={id} error={error} />
    </div>
  );
}

// ---- Translated text --------------------------------------------------------------------

export interface I18nTextFieldProps<T extends FieldValues> {
  control: Control<T>;
  /** Form path of the I18nText (record locale → text). */
  name: Path<T>;
  id: string;
  label: ReactNode;
  locales: readonly string[];
  defaultLocale: string;
  disabled?: boolean;
  multiline?: boolean;
  hint?: ReactNode;
}

/**
 * Shared I18nTextInput (one tab per enabled language, default first, "da
 * tradurre" marker) bound to an I18nText field; errors of every language below.
 */
export function I18nTextField<T extends FieldValues>({
  control,
  name,
  id,
  label,
  locales,
  defaultLocale,
  disabled,
  multiline,
  hint,
}: I18nTextFieldProps<T>) {
  const { field } = useController({ control, name });
  const { errors } = useFormState({ control, name });
  const errorText = useErrorText();
  const { t } = useTranslation();
  const messages = locales
    .map((l) => {
      const text = errorText(errorAt(errors, `${name}.${l}`));
      if (!text) return '';
      return l === defaultLocale
        ? text
        : `${t(`common.languages.${l}`, { defaultValue: l })}: ${text}`;
    })
    .filter(Boolean);
  const invalid = messages.length > 0;
  return (
    <UiField data-invalid={invalid || undefined} className="gap-2">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <I18nTextInput
        id={id}
        value={field.value as Record<string, string>}
        onChange={field.onChange}
        locales={locales}
        defaultLocale={defaultLocale}
        disabled={disabled}
        multiline={multiline}
        aria-invalid={invalid || undefined}
        aria-describedby={
          [invalid ? `${id}-error` : '', hint ? `${id}-hint` : ''].join(' ').trim() || undefined
        }
      />
      {hint && <FieldDescription id={`${id}-hint`}>{hint}</FieldDescription>}
      {invalid && <UiFieldError id={`${id}-error`}>{messages.join(' · ')}</UiFieldError>}
    </UiField>
  );
}
