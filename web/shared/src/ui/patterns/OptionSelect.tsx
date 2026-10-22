import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { cn } from '@arenile/shared/lib/utils';
import type { ReactNode } from 'react';

/** Radix Select cannot hold "": empty values travel as this sentinel. */
const NONE = '__none__';

export interface Option {
  value: string;
  label: ReactNode;
  icon?: ReactNode;
}

export interface OptionSelectProps {
  value: string;
  onChange: (v: string) => void;
  options: readonly Option[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
  name?: string;
  id?: string;
  onBlur?: () => void;
  'aria-label'?: string;
  'aria-labelledby'?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}

/** shadcn Select over a string value; an option with value "" is allowed. */
export function OptionSelect({
  value,
  onChange,
  options,
  placeholder,
  className,
  disabled,
  name,
  onBlur,
  ...aria
}: OptionSelectProps) {
  return (
    <Select
      value={value === '' ? NONE : value}
      onValueChange={(v) => onChange(v === NONE ? '' : v)}
      disabled={disabled}
      name={name}
    >
      <SelectTrigger className={cn('w-full min-w-0', className)} onBlur={onBlur} {...aria}>
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
  );
}
