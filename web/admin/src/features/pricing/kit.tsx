import { ApiProblem } from '@arenile/shared/api';
import {
  basisPointsToPercentText,
  formatScaled,
  parseEuroToCents,
  parsePercentToBasisPoints,
} from '@arenile/shared/lib/money';
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  Label,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  Skeleton,
  Spinner,
  Switch,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from '@arenile/shared/ui';
import {
  ConfirmDialog,
  MoneyInput,
  PageHeader,
  PercentInput,
  type ScaledInputProps,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  AlertCircleIcon,
  CalculatorIcon,
  CalendarCheckIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  EyeIcon,
  ListOrderedIcon,
  PencilIcon,
  RotateCwIcon,
  Table2Icon,
  Trash2Icon,
  XIcon,
} from 'lucide-react';
import { useId, useState, type ComponentProps, type FormEvent, type ReactNode } from 'react';
import type { Control, FieldPath, FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { problemMessageKey } from '../../lib/problem';
import type { Season } from '../configuration/api/resources';
import type { FormAlertState } from '../configuration/lib/formErrors';

/**
 * Building blocks shared by the pricing and policy pages, composed from the
 * shadcn kit (@arenile/shared/ui). No custom CSS: variants + utilities only.
 */

// ---------------------------------------------------------------------------
// Page layout
// ---------------------------------------------------------------------------

export function PricingLayout({
  title,
  description,
  actions,
  readOnly,
  establishmentId,
  seasonId,
  children,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  readOnly?: boolean;
  establishmentId: string;
  seasonId?: string;
  children: ReactNode;
}) {
  const { t } = useTranslation('configuration');
  return (
    <section aria-labelledby="page-title" className="mx-auto w-full max-w-6xl space-y-6">
      <PageHeader title={title} description={description} actions={actions} />
      <PricingNav establishmentId={establishmentId} seasonId={seasonId} />
      {readOnly && (
        <Alert role="note">
          <EyeIcon aria-hidden="true" />
          <AlertDescription>{t('readOnly')}</AlertDescription>
        </Alert>
      )}
      {children}
    </section>
  );
}

/** Links between the pricing tools (rules, simulator, matrix, coverage), shown as a segmented bar. */
export function PricingNav({
  establishmentId,
  seasonId,
}: {
  establishmentId: string;
  seasonId?: string;
}) {
  const { t } = useTranslation('pricing');
  const search = seasonId ? { seasonId } : {};
  const links = [
    { to: '/e/$establishmentId/pricing', label: t('nav.rules'), icon: ListOrderedIcon },
    {
      to: '/e/$establishmentId/pricing/simulator',
      label: t('nav.simulator'),
      icon: CalculatorIcon,
    },
    { to: '/e/$establishmentId/pricing/matrix', label: t('nav.matrix'), icon: Table2Icon },
    {
      to: '/e/$establishmentId/pricing/coverage',
      label: t('nav.coverage'),
      icon: CalendarCheckIcon,
    },
  ] as const;
  return (
    <nav
      aria-label={t('nav.label')}
      className="inline-flex max-w-full flex-wrap gap-1 rounded-lg bg-muted p-1"
    >
      {links.map((l) => (
        <Button
          key={l.to}
          asChild
          variant="ghost"
          size="sm"
          className="text-muted-foreground data-[status=active]:bg-background data-[status=active]:text-foreground data-[status=active]:shadow-sm"
        >
          <Link
            to={l.to}
            params={{ establishmentId }}
            search={search}
            activeOptions={{ exact: true, includeSearch: false }}
          >
            <l.icon aria-hidden="true" />
            {l.label}
          </Link>
        </Button>
      ))}
    </nav>
  );
}

export function SeasonSelect({
  seasons: items,
  season,
  onSelect,
}: {
  seasons: Season[];
  season: Season | undefined;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation('pricing');
  const id = useId();
  if (items.length === 0) return null;
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="text-muted-foreground">
        {t('season')}
      </Label>
      <Select value={season?.id ?? ''} onValueChange={onSelect}>
        <SelectTrigger id={id} className="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map((s) => (
            <SelectItem key={s.id} value={s.id}>
              {s.year} · {t(`seasonStatus.${s.status}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Query / form states
// ---------------------------------------------------------------------------

/** Loading skeleton or error alert (with retry) for a query. */
export function QueryState({
  isLoading,
  error,
  onRetry,
  rows = 3,
}: {
  isLoading: boolean;
  error: unknown;
  onRetry?: () => void;
  rows?: number;
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertCircleIcon aria-hidden="true" />
        <AlertTitle>{t('common.errors.title')}</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center gap-3">
          <span>{t(problemMessageKey(error instanceof ApiProblem ? error : undefined))}</span>
          {onRetry && (
            <Button size="sm" variant="outline" onClick={onRetry}>
              <RotateCwIcon aria-hidden="true" />
              {t('common.retry')}
            </Button>
          )}
        </AlertDescription>
      </Alert>
    );
  }
  if (isLoading) {
    return (
      <div role="status" className="space-y-2">
        <span className="sr-only">{t('common.loading')}</span>
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }
  return null;
}

/** Non-field errors of a form; for a stale version it offers to reload. */
export function FormErrorAlert({
  state,
  onReload,
}: {
  state: FormAlertState | null;
  onReload?: () => void;
}) {
  const { t } = useTranslation('configuration');
  if (!state) return null;
  return (
    <Alert variant="destructive">
      <AlertCircleIcon aria-hidden="true" />
      <AlertDescription className="space-y-2">
        {state.kind === 'conflict' && (
          <>
            <p>{t('common.errors.versionConflict', { ns: 'translation' })}</p>
            {onReload && (
              <Button size="sm" variant="outline" onClick={onReload}>
                {t('conflict.reload')}
              </Button>
            )}
          </>
        )}
        {state.kind === 'message' && <p>{t(state.key, { ns: 'translation' })}</p>}
        {state.kind === 'server' && (
          <ul className="list-inside list-disc">
            {state.messages.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  );
}

// ---------------------------------------------------------------------------
// Row actions
// ---------------------------------------------------------------------------

export function EditButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button variant="ghost" size="icon-sm" aria-label={label} onClick={onClick}>
      <PencilIcon aria-hidden="true" />
    </Button>
  );
}

/** Delete with confirmation; a failed delete keeps the dialog open and shows a toast. */
export function DeleteButton({
  label,
  message,
  onConfirm,
  withText,
}: {
  label: string;
  message: string;
  onConfirm: () => Promise<unknown>;
  /** Show "Elimina" next to the icon (e.g. in a form footer). */
  withText?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <ConfirmDialog
      title={t('common.actions.confirmDelete')}
      description={message}
      confirmLabel={t('common.actions.delete')}
      destructive
      onConfirm={async () => {
        try {
          await onConfirm();
        } catch (err) {
          const detail = err instanceof ApiProblem && err.detail ? err.detail : undefined;
          toast.error(detail ?? t(problemMessageKey(err)));
          throw err;
        }
      }}
      trigger={
        <Button
          variant="ghost"
          size={withText ? 'sm' : 'icon-sm'}
          aria-label={label}
          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
        >
          <Trash2Icon aria-hidden="true" />
          {withText && t('common.actions.delete')}
        </Button>
      }
    />
  );
}

export function RowActions({ children }: { children: ReactNode }) {
  return <div className="flex justify-end gap-0.5">{children}</div>;
}

// ---------------------------------------------------------------------------
// Editor sheet
// ---------------------------------------------------------------------------

/** Large right-side sheet hosting an editor form (rules, codes, agreements, policies). */
export function EditorSheet({
  open,
  onOpenChange,
  title,
  description,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 sm:max-w-3xl">
        <SheetHeader className="border-b">
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription className={description ? undefined : 'sr-only'}>
            {description ?? title}
          </SheetDescription>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}

/** Form inside an EditorSheet: scrolling body, sticky footer with cancel / save. */
export function SheetForm({
  onSubmit,
  onCancel,
  pending,
  submitLabel,
  footerStart,
  children,
}: {
  onSubmit: (e?: FormEvent) => void;
  onCancel: () => void;
  pending: boolean;
  submitLabel?: string;
  /** Left side of the footer (e.g. a delete button). */
  footerStart?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <form noValidate onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
      <div className="flex-1 space-y-8 overflow-y-auto p-4 sm:p-6">{children}</div>
      <SheetFooter className="flex-row items-center justify-between border-t bg-muted/30">
        <div>{footerStart}</div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onCancel}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={pending}>
            {pending && <Spinner />}
            {submitLabel ?? t('common.actions.save')}
          </Button>
        </div>
      </SheetFooter>
    </form>
  );
}

/** Titled block inside an editor form. */
export function EditorSection({
  title,
  description,
  children,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('space-y-4', className)}>
      <div className="space-y-1">
        <h3 className="text-sm font-semibold">{title}</h3>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  );
}

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

/** Radix Select cannot hold "": empty values travel as this sentinel. */
const NONE = '__none__';

export type Option = { value: string; label: string };

/** shadcn Select over a string value; an option with value "" is allowed. */
export function OptionSelect({
  value,
  onChange,
  options,
  placeholder,
  className,
  disabled,
  ...aria
}: {
  value: string;
  onChange: (v: string) => void;
  options: readonly Option[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
  id?: string;
  'aria-label'?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}) {
  return (
    <Select
      value={value === '' ? NONE : value}
      onValueChange={(v) => onChange(v === NONE ? '' : v)}
      disabled={disabled}
    >
      <SelectTrigger className={cn('w-full', className)} {...aria}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value || NONE} value={o.value === '' ? NONE : o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Multi-select on Popover + Command (search, check marks, clear). The trigger
 * shows the chosen items as badges; nothing chosen = `placeholder` ("Tutti").
 */
export function MultiSelect({
  value,
  onChange,
  options,
  placeholder,
  disabled,
  className,
  ...aria
}: {
  value: string[];
  onChange: (v: string[]) => void;
  options: readonly Option[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}) {
  const { t } = useTranslation('pricing');
  const [open, setOpen] = useState(false);
  const selected = options.filter((o) => value.includes(o.value));
  const toggle = (v: string) => {
    const next = value.includes(v) ? value.filter((x) => x !== v) : [...value, v];
    // Keep the options' order, independent of click order.
    onChange(options.map((o) => o.value).filter((o) => next.includes(o)));
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn('h-auto min-h-9 w-full justify-between px-3 py-1.5 font-normal', className)}
          {...aria}
        >
          <span className="flex min-w-0 flex-wrap gap-1">
            {selected.length === 0 && (
              <span className="text-muted-foreground">{placeholder ?? t('multi.all')}</span>
            )}
            {selected.slice(0, 4).map((o) => (
              <Badge key={o.value} variant="secondary">
                {o.label}
              </Badge>
            ))}
            {selected.length > 4 && (
              <Badge variant="outline">{t('multi.more', { count: selected.length - 4 })}</Badge>
            )}
          </span>
          <ChevronsUpDownIcon aria-hidden="true" className="text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) min-w-56 p-0" align="start">
        <Command>
          {options.length > 6 && <CommandInput placeholder={t('multi.search')} />}
          <CommandList>
            <CommandEmpty>{t('ui.noResults', { ns: 'translation' })}</CommandEmpty>
            <CommandGroup>
              {options.map((o) => {
                const on = value.includes(o.value);
                return (
                  <CommandItem
                    key={o.value}
                    value={`${o.label} ${o.value}`}
                    onSelect={() => toggle(o.value)}
                    aria-selected={on}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'flex size-4 items-center justify-center rounded-sm border',
                        on ? 'border-primary bg-primary text-primary-foreground' : 'opacity-60',
                      )}
                    >
                      {on && <CheckIcon className="size-3" />}
                    </span>
                    {o.label}
                  </CommandItem>
                );
              })}
            </CommandGroup>
            {value.length > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem onSelect={() => onChange([])} className="justify-center">
                    <XIcon aria-hidden="true" />
                    {t('multi.clear')}
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/** Few fixed choices (weekdays, slots, channels) as a multiple ToggleGroup. */
export function MultiToggle({
  value,
  onChange,
  options,
  label,
  disabled,
  ...aria
}: {
  value: string[];
  onChange: (v: string[]) => void;
  options: readonly Option[];
  /** Accessible name of the group. */
  label: string;
  disabled?: boolean;
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}) {
  return (
    <ToggleGroup
      type="multiple"
      variant="outline"
      size="sm"
      aria-label={label}
      disabled={disabled}
      value={value}
      onValueChange={(next) =>
        onChange(options.map((o) => o.value).filter((o) => next.includes(o)))
      }
      className="flex-wrap"
      {...aria}
    >
      {options.map((o) => (
        <ToggleGroupItem key={o.value} value={o.value}>
          {o.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

/** One of a few choices as a single ToggleGroup (never empty). */
export function SingleToggle({
  value,
  onChange,
  options,
  label,
  disabled,
  ...aria
}: {
  value: string;
  onChange: (v: string) => void;
  options: readonly Option[];
  label: string;
  disabled?: boolean;
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label={label}
      disabled={disabled}
      value={value}
      onValueChange={(v) => v && onChange(v)}
      className="flex-wrap"
      {...aria}
    >
      {options.map((o) => (
        <ToggleGroupItem key={o.value} value={o.value}>
          {o.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

type TextScaledProps = Omit<ScaledInputProps, 'value' | 'onChange'> & {
  /** Euro amount as typed in the form schema ("12,50", "-2,00"). */
  value: string;
  onChange: (v: string) => void;
};

/** MoneyInput bound to the form's euro text ("12,50"); invalid input clears the value. */
export function EuroTextInput({ value, onChange, allowNegative, ...props }: TextScaledProps) {
  return (
    <MoneyInput
      {...props}
      allowNegative={allowNegative}
      value={parseEuroToCents(value, { allowNegative })}
      onChange={(c) => onChange(c == null ? '' : formatScaled(c, 2, { grouping: false }))}
    />
  );
}

/** PercentInput bound to the form's percent text ("12,5"). */
export function PercentTextInput({ value, onChange, ...props }: TextScaledProps) {
  return (
    <PercentInput
      {...props}
      value={parsePercentToBasisPoints(value)}
      onChange={(bp) => onChange(bp == null ? '' : basisPointsToPercentText(bp))}
    />
  );
}

// ---------------------------------------------------------------------------
// Form rows
// ---------------------------------------------------------------------------

/** Switch with label and description bound to a boolean form field. */
export function SwitchRow<T extends FieldValues>({
  control,
  name,
  label,
  description,
  disabled,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className="flex flex-row items-start justify-between gap-4 rounded-lg border p-3">
          <div className="space-y-1">
            <FormLabel>{label}</FormLabel>
            {description && <FormDescription>{description}</FormDescription>}
          </div>
          <FormControl>
            <Switch
              checked={!!field.value}
              onCheckedChange={field.onChange}
              onBlur={field.onBlur}
              disabled={disabled}
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
}

/** Text input field (labels, hint, translated error). */
export function TextRow<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  inputProps,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: ReactNode;
  description?: ReactNode;
  className?: string;
  inputProps?: ComponentProps<'input'>;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className={className}>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input {...inputProps} {...field} value={(field.value as string) ?? ''} />
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
