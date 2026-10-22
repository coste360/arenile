import type { ReactNode } from 'react';
import { useController, type Control, type FieldValues, type Path } from 'react-hook-form';
import { Checkbox, CheckboxGroup } from './fields';

export interface MultiCheckProps<T extends FieldValues> {
  control: Control<T>;
  name: Path<T>;
  id: string;
  legend: ReactNode;
  options: { value: string; label: ReactNode; icon?: ReactNode }[];
  disabled?: boolean;
  hint?: ReactNode;
}

/**
 * Checkbox group bound to a string[] field. Controlled (unlike registering
 * every box with the same name) so a group with a single option still yields
 * an array.
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
    // Keep the options' order, independent of click order.
    field.onChange(options.map((o) => o.value).filter((o) => next.includes(o)));
  };
  return (
    <CheckboxGroup id={id} legend={legend} error={fieldState.error}>
      {options.map((o) => (
        <Checkbox
          key={o.value}
          id={`${id}-${o.value}`}
          checked={value.includes(o.value)}
          disabled={disabled}
          onChange={(e) => toggle(o.value, e.target.checked)}
          onBlur={field.onBlur}
          label={
            o.icon ? (
              <span className="inline-flex items-center gap-1">
                {o.icon}
                {o.label}
              </span>
            ) : (
              o.label
            )
          }
        />
      ))}
      {hint && <p className="w-full text-xs text-slate-600">{hint}</p>}
    </CheckboxGroup>
  );
}
