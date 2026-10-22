import type { FieldError as RhfFieldError } from 'react-hook-form';

/** aria-invalid/aria-describedby for an input whose error is rendered by <FieldError id={id} />. */
export function fieldAria(id: string, error: RhfFieldError | undefined, hint = false) {
  const describedBy = [error ? `${id}-error` : null, hint ? `${id}-hint` : null]
    .filter(Boolean)
    .join(' ');
  return {
    id,
    'aria-invalid': error ? (true as const) : undefined,
    'aria-describedby': describedBy || undefined,
  };
}

/** Walks a nested RHF errors object by dotted path. */
export function errorAt(errors: unknown, path: string): RhfFieldError | undefined {
  let cur: unknown = errors;
  for (const part of path.split('.')) {
    if (cur === null || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  if (cur && typeof cur === 'object' && 'message' in cur) return cur as RhfFieldError;
  return undefined;
}
