import { cn } from '@arenile/shared/lib/utils';
import { Button } from '@arenile/shared/ui/button';
import { ButtonGroup } from '@arenile/shared/ui/button-group';
import { MinusIcon, PlusIcon } from 'lucide-react';
import { useEffect, useId, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';

export interface QuantityStepperProps {
  value: number;
  onChange: (value: number) => void;
  /** Accessible name of the quantity (e.g. the extra's name). */
  label: string;
  min?: number;
  max: number;
  step?: number;
  /** Accessible names of the buttons; default "Diminuisci {label}" / "Aumenta {label}". */
  decreaseLabel?: string;
  increaseLabel?: string;
  /** `lg`: 44 px touch targets (public pages, spec 12.6); `default`: 36 px (panel forms). */
  size?: 'default' | 'lg';
  disabled?: boolean;
  id?: string;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
  className?: string;
}

/** Clamps `n` into [min, max] on the step grid; NaN or missing values fall back to `min`. */
export function clampQuantity(n: number, min: number, max: number, step = 1): number {
  if (!Number.isFinite(n)) return min;
  const upper = Math.max(min, max);
  const snapped = min + Math.round((n - min) / step) * step;
  return Math.min(upper, Math.max(min, snapped));
}

/**
 * − value + quantity control (ARIA spinbutton). The value is always visible
 * and the control has a fixed width, so a long label next to it never
 * squeezes it. Arrow keys change the value, Home/End jump to the bounds, and
 * the buttons are disabled at the bounds: the value can never leave
 * [min, max].
 */
export function QuantityStepper({
  value,
  onChange,
  label,
  min = 0,
  max,
  step = 1,
  decreaseLabel,
  increaseLabel,
  size = 'default',
  disabled = false,
  id,
  className,
  ...aria
}: QuantityStepperProps) {
  const { t } = useTranslation();
  const upper = Math.max(min, max);
  const current = clampQuantity(value, min, upper, step);
  const autoId = useId();
  const valueId = id ?? autoId;
  // Never show a number the form does not hold: when the bounds change (e.g.
  // availability drops after a refetch) the clamped value is reported back.
  useEffect(() => {
    if (!Object.is(value, current)) onChange(current);
  }, [value, current, onChange]);
  const set = (n: number) => {
    const next = clampQuantity(n, min, upper, step);
    if (next !== current) onChange(next);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (disabled) return;
    const keys: Record<string, number> = {
      ArrowUp: current + step,
      ArrowRight: current + step,
      ArrowDown: current - step,
      ArrowLeft: current - step,
      PageUp: current + step * 10,
      PageDown: current - step * 10,
      Home: min,
      End: upper,
    };
    const next = keys[e.key];
    if (next === undefined) return;
    e.preventDefault();
    set(next);
  };
  const button = size === 'lg' ? 'size-11' : 'size-9';
  return (
    <ButtonGroup className={cn('shrink-0', className)}>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={button}
        aria-label={decreaseLabel ?? t('ui.quantity.decrease', { name: label })}
        aria-controls={valueId}
        disabled={disabled || current <= min}
        onClick={() => set(current - step)}
      >
        <MinusIcon aria-hidden="true" />
      </Button>
      <div
        id={valueId}
        role="spinbutton"
        // Announces the new value after a click on − / + (focus stays on the button).
        aria-live="polite"
        tabIndex={disabled ? -1 : 0}
        aria-label={label}
        aria-valuenow={current}
        aria-valuemin={min}
        aria-valuemax={upper}
        aria-disabled={disabled || undefined}
        aria-invalid={aria['aria-invalid'] || undefined}
        aria-describedby={aria['aria-describedby']}
        onKeyDown={onKeyDown}
        className={cn(
          'flex w-11 shrink-0 items-center justify-center border bg-background text-base font-semibold tabular-nums shadow-xs outline-none dark:border-input dark:bg-input/30',
          'focus-visible:z-10 focus-visible:ring-3 focus-visible:ring-ring/50',
          'aria-invalid:border-destructive aria-invalid:text-destructive',
          disabled && 'opacity-50',
        )}
      >
        {current}
      </div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={button}
        aria-label={increaseLabel ?? t('ui.quantity.increase', { name: label })}
        aria-controls={valueId}
        disabled={disabled || current >= upper}
        onClick={() => set(current + step)}
      >
        <PlusIcon aria-hidden="true" />
      </Button>
    </ButtonGroup>
  );
}
