import { Field, FieldDescription, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { cn } from '@arenile/shared/lib/utils';
import type { ReactNode } from 'react';
import type { FieldError as RhfFieldError } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export interface CashFieldProps {
  id: string;
  label: ReactNode;
  error?: RhfFieldError;
  hint?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** shadcn Field with label, hint and translated zod error (keys of the cash namespace). */
export function CashField({ id, label, error, hint, className, children }: CashFieldProps) {
  const { t } = useTranslation('cash');
  return (
    <Field data-invalid={error ? true : undefined} className={cn('gap-2', className)}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {children}
      {hint && <FieldDescription id={`${id}-hint`}>{hint}</FieldDescription>}
      {error?.message && <FieldError id={`${id}-error`}>{t(error.message)}</FieldError>}
    </Field>
  );
}
