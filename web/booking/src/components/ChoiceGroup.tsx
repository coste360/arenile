import { cn } from '@arenile/shared/lib/utils';
import { Button } from '@arenile/shared/ui/button';
import { useRef, type KeyboardEvent, type ReactNode } from 'react';

export interface Choice<T extends string> {
  value: T;
  label: ReactNode;
  disabled?: boolean;
}

export interface ChoiceGroupProps<T extends string> {
  /** Id of the element naming the group. */
  labelledBy: string;
  value: T | null;
  options: readonly Choice<T>[];
  onChange: (value: T) => void;
  className?: string;
}

/**
 * Single choice as a row of shadcn Buttons with radio semantics (WAI-ARIA
 * radio group: one tab stop, arrows move and select). Used instead of the
 * Radix RadioGroup / ToggleGroup, whose styles stay out of the public page CSS.
 * Buttons are 44 px tall (touch targets, spec 12.6).
 */
export function ChoiceGroup<T extends string>({
  labelledBy,
  value,
  options,
  onChange,
  className,
}: ChoiceGroupProps<T>) {
  const ref = useRef<HTMLDivElement>(null);
  const enabled = options.filter((o) => !o.disabled);
  const current = enabled.find((o) => o.value === value) ?? enabled[0];

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const step =
      e.key === 'ArrowRight' || e.key === 'ArrowDown'
        ? 1
        : e.key === 'ArrowLeft' || e.key === 'ArrowUp'
          ? -1
          : 0;
    if (!step || enabled.length === 0) return;
    e.preventDefault();
    const at = enabled.findIndex((o) => o.value === current?.value);
    const next = enabled[(at + step + enabled.length) % enabled.length];
    if (!next) return;
    onChange(next.value);
    ref.current?.querySelector<HTMLButtonElement>(`[data-value="${next.value}"]`)?.focus();
  };

  return (
    <div
      ref={ref}
      role="radiogroup"
      aria-labelledby={labelledBy}
      onKeyDown={onKeyDown}
      className={cn('flex flex-wrap gap-2', className)}
    >
      {options.map((o) => {
        const checked = o.value === value;
        return (
          <Button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={checked}
            data-value={o.value}
            disabled={o.disabled}
            tabIndex={o.value === current?.value ? 0 : -1}
            variant={checked ? 'default' : 'outline'}
            className="h-11 min-w-11"
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </Button>
        );
      })}
    </div>
  );
}
