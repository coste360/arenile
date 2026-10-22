import {
  basisPointsToPercentText,
  formatScaled,
  parseEuroToCents,
  parsePercentToBasisPoints,
} from '@arenile/shared/lib/money';
import {
  Alert,
  AlertDescription,
  Button,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from '@arenile/shared/ui';
import {
  MoneyInput,
  PageHeader,
  PercentInput,
  type Option,
  type ScaledInputProps,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  AlertCircleIcon,
  CalculatorIcon,
  CalendarCheckIcon,
  EyeIcon,
  ListOrderedIcon,
  Table2Icon,
} from 'lucide-react';
import { useId, type ComponentProps, type ReactNode } from 'react';
import type { Control, FieldPath, FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { Season } from '../configuration/api/resources';
import type { FormAlertState } from '../configuration/lib/formErrors';

/**
 * Building blocks specific to the pricing and policy pages (layout, season
 * selector, toggles, text-bound money inputs, form rows). Generic pieces
 * (QueryState, RowActions, selects, dialogs) live in @arenile/shared/ui/patterns.
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

/**
 * MoneyInput bound to the form's euro text ("12,50"). Unreadable input is kept
 * as typed, so the zod schema rejects it and the error stays visible (an empty
 * string would silently drop an optional amount).
 */
export function EuroTextInput({ value, onChange, allowNegative, ...props }: TextScaledProps) {
  return (
    <MoneyInput
      {...props}
      allowNegative={allowNegative}
      value={parseEuroToCents(value, { allowNegative })}
      onChange={(c, text) => onChange(c == null ? text : formatScaled(c, 2, { grouping: false }))}
    />
  );
}

/** PercentInput bound to the form's percent text ("12,5"); unreadable input is kept as typed. */
export function PercentTextInput({ value, onChange, ...props }: TextScaledProps) {
  return (
    <PercentInput
      {...props}
      value={parsePercentToBasisPoints(value)}
      onChange={(bp, text) => onChange(bp == null ? text : basisPointsToPercentText(bp))}
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
