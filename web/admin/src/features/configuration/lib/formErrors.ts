import { ApiProblem } from '@arenile/shared/api';
import type { FieldValues, Path, UseFormSetError } from 'react-hook-form';

/** A non-field problem shown above the form. */
export type FormAlertState =
  { kind: 'conflict' } | { kind: 'message'; key: string } | { kind: 'server'; messages: string[] };

/**
 * Normalises a server field path to react-hook-form dotted notation:
 * "ranges[0].from", "ranges/0/from" and "/ranges/0/from" all become "ranges.0.from".
 */
export function normalizeFieldPath(field: string): string {
  return field
    .replace(/\[(\d+)\]/g, '.$1')
    .replace(/\//g, '.')
    .replace(/^\.+/, '')
    .replace(/\.{2,}/g, '.');
}

export interface ApplyFieldErrorsOptions {
  /** Top-level keys of the form values: only paths starting with one of them are set on fields. */
  formKeys: readonly string[];
  /** Server path (or prefix) → form path, for fields named differently in the form. */
  aliases?: Record<string, string>;
}

/**
 * Maps a problem+json `errors[]` onto react-hook-form fields (as `server`
 * errors, whose message is the server text, not an i18n key). Returns the
 * messages that matched no field so the caller can show them in an alert.
 */
export function applyFieldErrors<T extends FieldValues>(
  problem: ApiProblem,
  setError: UseFormSetError<T>,
  options: ApplyFieldErrorsOptions,
): string[] {
  const unmatched: string[] = [];
  let focused = false;
  for (const fe of problem.fieldErrors) {
    let path = normalizeFieldPath(fe.field);
    for (const [from, to] of Object.entries(options.aliases ?? {})) {
      if (path === from || path.startsWith(`${from}.`)) {
        path = to + path.slice(from.length);
        break;
      }
    }
    const head = path.split('.')[0] ?? '';
    if (head && options.formKeys.includes(head)) {
      setError(path as Path<T>, { type: 'server', message: fe.message }, { shouldFocus: !focused });
      focused = true;
    } else {
      unmatched.push(fe.field ? `${fe.field}: ${fe.message}` : fe.message);
    }
  }
  return unmatched;
}

/** True for a stale If-Match (412 version_conflict). */
export function isVersionConflict(error: unknown): boolean {
  return error instanceof ApiProblem && (error.type === 'version_conflict' || error.status === 412);
}

/**
 * Turns a mutation failure into form state: field errors on inputs, plus the
 * alert to show (or null when everything was attached to a field).
 */
export function problemToFormState<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T>,
  options: ApplyFieldErrorsOptions,
): FormAlertState | null {
  if (isVersionConflict(error)) return { kind: 'conflict' };
  if (!(error instanceof ApiProblem)) return { kind: 'message', key: 'common.errors.generic' };
  if (error.fieldErrors.length > 0) {
    const rest = applyFieldErrors(error, setError, options);
    if (rest.length === 0) return null;
    return { kind: 'server', messages: rest };
  }
  if (error.isNetworkError) return { kind: 'message', key: 'common.errors.network' };
  switch (error.type) {
    case 'forbidden':
    case 'csrf_failed':
      return { kind: 'message', key: 'common.errors.forbidden' };
    case 'conflict':
      return error.detail
        ? { kind: 'server', messages: [error.detail] }
        : { kind: 'message', key: 'common.errors.conflict' };
    case 'not_found':
      return { kind: 'message', key: 'common.errors.notFound' };
    case 'not_implemented':
      return { kind: 'message', key: 'common.errors.notImplemented' };
    case 'rate_limited':
      return { kind: 'message', key: 'common.errors.tooManyRequests' };
    case 'validation_failed':
    case 'bad_request':
      return error.detail
        ? { kind: 'server', messages: [error.detail] }
        : { kind: 'message', key: 'common.errors.validation' };
    default:
      return { kind: 'message', key: 'common.errors.generic' };
  }
}
