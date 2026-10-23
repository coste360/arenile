import type { FieldError } from 'react-hook-form';

/** aria attributes linking a control to its hint and error (ids from CashField). */
export function fieldAria(id: string, error: FieldError | undefined, hint = false) {
  const describedBy = [hint && `${id}-hint`, error && `${id}-error`].filter(Boolean).join(' ');
  return {
    id,
    'aria-invalid': error ? true : undefined,
    'aria-describedby': describedBy || undefined,
  } as const;
}
