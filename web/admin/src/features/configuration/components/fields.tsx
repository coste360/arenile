import { cn, Label } from '@arenile/shared/ui';
import {
  forwardRef,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react';
import type { FieldError as RhfFieldError } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

/**
 * Error under an input. Client errors carry an i18n key; errors mapped from
 * problem+json (`type: "server"`) carry the server text verbatim.
 */
export function FieldError({ id, error }: { id: string; error: RhfFieldError | undefined }) {
  const { t } = useTranslation();
  if (!error?.message) return null;
  return (
    <p id={`${id}-error`} className="text-sm text-red-700">
      {error.type === 'server' ? error.message : t(error.message)}
    </p>
  );
}

export interface FieldProps {
  id: string;
  label: ReactNode;
  error?: RhfFieldError;
  hint?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** Label + control + hint + error. The control must use `fieldAria(id, error, !!hint)`. */
export function Field({ id, label, error, hint, className, children }: FieldProps) {
  return (
    <div className={cn('space-y-1.5', className)}>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && (
        <p id={`${id}-hint`} className="text-xs text-slate-600">
          {hint}
        </p>
      )}
      <FieldError id={id} error={error} />
    </div>
  );
}

const controlClass =
  'w-full rounded-[var(--ar-radius)] border border-slate-300 bg-white px-3 text-sm text-slate-900 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--ar-primary)] disabled:cursor-not-allowed disabled:opacity-60 aria-[invalid=true]:border-red-600';

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, ...props }, ref) => (
    <select ref={ref} className={cn(controlClass, 'h-10', className)} {...props} />
  ),
);
Select.displayName = 'Select';

export const Textarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea ref={ref} className={cn(controlClass, 'min-h-20 py-2', className)} {...props} />
));
Textarea.displayName = 'Textarea';

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label: ReactNode;
  description?: ReactNode;
}

/** Native checkbox with its label (also used as a toggle). */
export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, description, className, id, ...props }, ref) => (
    <label htmlFor={id} className={cn('flex items-start gap-2 text-sm', className)}>
      <input
        ref={ref}
        id={id}
        type="checkbox"
        className="mt-0.5 h-4 w-4 shrink-0 accent-[var(--ar-primary)] disabled:opacity-60"
        {...props}
      />
      <span>
        <span className="font-medium text-slate-800">{label}</span>
        {description && <span className="block text-xs text-slate-600">{description}</span>}
      </span>
    </label>
  ),
);
Checkbox.displayName = 'Checkbox';

/** Group of checkboxes with a legend. */
export function CheckboxGroup({
  legend,
  error,
  id,
  children,
}: {
  legend: ReactNode;
  error?: RhfFieldError;
  id: string;
  children: ReactNode;
}) {
  return (
    <fieldset className="space-y-2" aria-describedby={error ? `${id}-error` : undefined}>
      <legend className="text-sm font-medium text-slate-800">{legend}</legend>
      <div className="flex flex-wrap gap-x-4 gap-y-2">{children}</div>
      <FieldError id={id} error={error} />
    </fieldset>
  );
}
