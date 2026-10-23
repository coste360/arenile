import type { FieldValues, Path, UseFormSetError } from 'react-hook-form';
import { ApiProblem } from './client';

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

/** Where a server field error goes in the form, and how it reads to a person. */
export interface FieldTarget {
  /** react-hook-form field name; omit to show the error only in the alert. */
  name?: string;
  /** Human label of the field ("Lettino aggiuntivo"), used in the alert. */
  label?: string;
  /** Message replacing the server text (e.g. a translated "Massimo 2"). */
  message?: string;
}

export interface MapProblemOptions {
  /** Top-level keys of the form values: paths starting with one of them go to that field. */
  formKeys?: readonly string[];
  /** Server path (or prefix) → form path, for fields named differently in the form. */
  aliases?: Record<string, string>;
  /**
   * Custom resolution of a normalised server path ("lines.0.extras.1.quantity"),
   * tried first. Return undefined to fall back to `formKeys`/`aliases`.
   */
  resolve?: (path: string, message: string) => FieldTarget | undefined;
  /** Human label of a path that `resolve` did not label (e.g. from a field label map). */
  label?: (path: string) => string | undefined;
}

/** One server field error, ready to show: never a raw path. */
export interface MappedFieldError {
  /** Form field it was attached to, if any. */
  field?: string;
  /** "Lettino aggiuntivo: massimo 2", or just the message when no label is known. */
  text: string;
}

export interface MappedProblem {
  /** Every field error in plain words (attached to a field or not), in server order. */
  errors: MappedFieldError[];
  /** The ones not attached to any field (to list in the form alert). */
  unmatched: string[];
}

/** "Massimo 2." → "massimo 2" so it reads after "Label: " (acronyms such as "IBAN" stay). */
function lowerFirst(message: string): string {
  const trimmed = message.trim().replace(/\.$/, '');
  const second = trimmed.charAt(1);
  if (second && second === second.toLocaleUpperCase()) return trimmed;
  return trimmed.charAt(0).toLocaleLowerCase() + trimmed.slice(1);
}

/** "Label: message" when the label is known, else the bare message (no raw path). */
export function fieldErrorText(label: string | undefined, message: string): string {
  return label ? `${label}: ${lowerFirst(message)}` : message;
}

/**
 * Maps the `errors[]` of a problem+json (validation_failed) onto
 * react-hook-form: each server path becomes a form field error (set as
 * `server`, focusing the first one) when the form has that field, and every
 * error gets a human text for the form alert. Raw server paths such as
 * `lines[0].extras[0].quantity` are never part of the output.
 */
export function mapProblemToForm<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T> | undefined,
  options: MapProblemOptions = {},
): MappedProblem {
  const result: MappedProblem = { errors: [], unmatched: [] };
  if (!(error instanceof ApiProblem)) return result;
  let focused = false;
  for (const fe of error.fieldErrors) {
    let path = normalizeFieldPath(fe.field);
    const custom = path ? options.resolve?.(path, fe.message) : undefined;
    let name = custom?.name;
    if (!custom && path) {
      for (const [from, to] of Object.entries(options.aliases ?? {})) {
        if (path === from || path.startsWith(`${from}.`)) {
          path = to + path.slice(from.length);
          break;
        }
      }
      const head = path.split('.')[0] ?? '';
      if (head && options.formKeys?.includes(head)) name = path;
    }
    const message = custom?.message ?? fe.message;
    const label = custom?.label ?? (path ? options.label?.(path) : undefined);
    const text = fieldErrorText(label, message);
    // One entry per field and text: per-spot extras repeat the same error on every line.
    if (result.errors.some((e) => e.field === (setError ? name : undefined) && e.text === text)) {
      continue;
    }
    if (name && setError) {
      setError(name as Path<T>, { type: 'server', message }, { shouldFocus: !focused });
      focused = true;
      result.errors.push({ field: name, text });
    } else {
      result.errors.push({ text });
      result.unmatched.push(text);
    }
  }
  return result;
}
