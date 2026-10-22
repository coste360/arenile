import type { Establishment } from '@arenile/shared/api';
import { useState } from 'react';
import type { FieldValues, UseFormReturn } from 'react-hook-form';
import { problemToFormState, type FormAlertState } from '../lib/formErrors';
import {
  useEstablishment,
  useUpdateEstablishment,
  type EstablishmentUpdate,
} from './establishment';

export interface EstablishmentSectionOptions<T extends FieldValues> {
  establishmentId: string;
  form: UseFormReturn<T>;
  /**
   * Version the page started editing from. Background refetches do not move
   * it, so a change made by someone else is detected (412) instead of being
   * silently overwritten; our own saves move it forward via `onVersion`.
   */
  version: number;
  onVersion: (version: number) => void;
  toPatch: (values: T, current: Establishment) => EstablishmentUpdate;
  toValues: (est: Establishment) => T;
  /** Server field path → form field, see applyFieldErrors. */
  aliases?: Record<string, string>;
  /** Called after a successful save (not after a reload). */
  onSaved?: () => void;
}

/** Submit/conflict/reload logic shared by the establishment settings sections. */
export function useEstablishmentSection<T extends FieldValues>(
  opts: EstablishmentSectionOptions<T>,
) {
  const { establishmentId, form, version, onVersion, toPatch, toValues, aliases, onSaved } = opts;
  const query = useEstablishment(establishmentId);
  const update = useUpdateEstablishment(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [saved, setSaved] = useState(false);

  const submit = form.handleSubmit(async (values) => {
    const current = query.data;
    if (!current) return;
    setAlert(null);
    setSaved(false);
    try {
      const est = await update.mutateAsync({ version, patch: toPatch(values, current) });
      onVersion(est.version);
      form.reset(toValues(est));
      setSaved(true);
      onSaved?.();
    } catch (err) {
      setAlert(
        problemToFormState(err, form.setError, {
          formKeys: Object.keys(form.getValues()),
          aliases,
        }),
      );
    }
  });

  /** After a 412: load the server version and restart editing from it. */
  const reload = async () => {
    const res = await query.refetch();
    if (res.data) {
      onVersion(res.data.version);
      form.reset(toValues(res.data));
      setAlert(null);
    }
  };

  return {
    submit,
    reload: () => void reload(),
    alert,
    saved: saved && !form.formState.isDirty,
    pending: update.isPending,
  };
}
