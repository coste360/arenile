import { cn } from '@arenile/shared/lib/utils';
import { Button } from '@arenile/shared/ui/button';
import { MinusIcon, PlusIcon } from 'lucide-react';
import { useId } from 'react';

export interface NumberStepperProps {
  value: number;
  min?: number;
  max: number;
  onChange: (value: number) => void;
  /** Accessible names of the buttons ("Diminuisci Ombrelloni"). */
  decreaseLabel: string;
  increaseLabel: string;
  /** Accessible name of the value. */
  label: string;
  className?: string;
}

/** − value + quantity selector with 44 px buttons and a live value (spec 12.3, 12.6). */
export function NumberStepper({
  value,
  min = 0,
  max,
  onChange,
  decreaseLabel,
  increaseLabel,
  label,
  className,
}: NumberStepperProps) {
  const id = useId();
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-11"
        aria-label={decreaseLabel}
        aria-controls={id}
        disabled={value <= min}
        onClick={() => onChange(Math.max(min, value - 1))}
      >
        <MinusIcon aria-hidden="true" />
      </Button>
      <output
        id={id}
        aria-label={label}
        aria-live="polite"
        className="min-w-8 text-center text-lg font-semibold tabular-nums"
      >
        {value}
      </output>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-11"
        aria-label={increaseLabel}
        aria-controls={id}
        disabled={value >= max}
        onClick={() => onChange(Math.min(max, value + 1))}
      >
        <PlusIcon aria-hidden="true" />
      </Button>
    </div>
  );
}
