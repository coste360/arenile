import type { ApiProblem } from '@arenile/shared/api';
import type { UseMutationResult } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { FieldValues, UseFormReturn } from 'react-hook-form';
import { problemToFormState, type FormAlertState } from '../lib/formErrors';
import type { UpdateVars, Versioned } from './resource';

/** Open/close state of a create/edit dialog. */
export function useEditor<TItem>() {
  const [state, setState] = useState<{ open: boolean; item?: TItem; nonce: number }>({
    open: false,
    nonce: 0,
  });
  return {
    open: state.open,
    item: state.item,
    /** Changes at every opening: use as `key` to remount the form with fresh defaults. */
    key: state.nonce,
    create: () => setState((s) => ({ open: true, item: undefined, nonce: s.nonce + 1 })),
    edit: (item: TItem) => setState((s) => ({ open: true, item, nonce: s.nonce + 1 })),
    close: () => setState((s) => ({ ...s, open: false })),
    setOpen: (open: boolean) => setState((s) => ({ ...s, open })),
  };
}

export interface CrudSubmitOptions<T extends FieldValues, TItem extends Versioned, TInput> {
  form: UseFormReturn<T>;
  /** Item being edited; undefined when creating. */
  item?: TItem;
  create: UseMutationResult<TItem, ApiProblem, TInput>;
  update: UseMutationResult<TItem, ApiProblem, UpdateVars<TInput>>;
  toInput: (values: T) => TInput;
  onDone: (saved: TItem) => void;
  aliases?: Record<string, string>;
}

/**
 * Create (POST + Idempotency-Key) or replace (PUT + If-Match with the version
 * the dialog was opened with) and map problem+json onto the form.
 */
export function useCrudSubmit<T extends FieldValues, TItem extends Versioned, TInput>(
  opts: CrudSubmitOptions<T, TItem, TInput>,
) {
  const { form, item, create, update, toInput, onDone, aliases } = opts;
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const { t } = useTranslation('configuration');
  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const input = toInput(values);
      const saved = item
        ? await update.mutateAsync({ id: item.id, version: item.version, input })
        : await create.mutateAsync(input);
      toast.success(t(item ? 'toast.saved' : 'toast.created'));
      onDone(saved);
    } catch (err) {
      setAlert(
        problemToFormState(err, form.setError, {
          formKeys: Object.keys(form.getValues()),
          aliases,
        }),
      );
    }
  });
  return { submit, alert, pending: create.isPending || update.isPending };
}
