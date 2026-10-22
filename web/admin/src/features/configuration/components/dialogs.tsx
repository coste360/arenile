import { Button, Dialog, DialogContent, DialogDescription, DialogTitle } from '@arenile/shared/ui';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../../../lib/problem';
import { isVersionConflict } from '../lib/formErrors';

export interface FormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  wide?: boolean;
}

/** Modal hosting a create/edit form. */
export function FormDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  wide,
}: FormDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        closeLabel={t('common.close')}
        className={wide ? 'max-h-[90vh] max-w-3xl overflow-y-auto' : 'max-h-[90vh] overflow-y-auto'}
      >
        <DialogTitle>{title}</DialogTitle>
        {description ? (
          <DialogDescription>{description}</DialogDescription>
        ) : (
          <DialogDescription className="sr-only">{title}</DialogDescription>
        )}
        {children}
      </DialogContent>
    </Dialog>
  );
}

export interface ConfirmDeleteProps {
  /** Accessible name of the trigger, e.g. "Elimina stagione 2027". */
  label: string;
  /** Text of the confirmation. */
  message: string;
  onConfirm: () => Promise<unknown>;
  disabled?: boolean;
}

/** Delete button with a confirmation dialog; shows the problem when the delete fails. */
export function ConfirmDelete({ label, message, onConfirm, disabled }: ConfirmDeleteProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const confirm = async () => {
    setPending(true);
    setError(null);
    try {
      await onConfirm();
      setOpen(false);
    } catch (err) {
      setError(err);
    } finally {
      setPending(false);
    }
  };

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        aria-label={label}
        disabled={disabled}
        onClick={() => {
          setError(null);
          setOpen(true);
        }}
        className="text-red-700 hover:bg-red-50"
      >
        {t('common.actions.delete')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent closeLabel={t('common.close')}>
          <DialogTitle>{t('common.actions.confirmDelete')}</DialogTitle>
          <DialogDescription>{message}</DialogDescription>
          {error ? (
            <p role="alert" className="text-sm text-red-700">
              {isVersionConflict(error)
                ? t('common.errors.versionConflict')
                : error instanceof Error && 'detail' in error && error.detail
                  ? String(error.detail)
                  : t(problemMessageKey(error))}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('common.actions.cancel')}
            </Button>
            <Button variant="destructive" onClick={() => void confirm()} disabled={pending}>
              {t('common.actions.delete')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

/** Save / cancel row at the bottom of a dialog form. */
export function FormActions({
  onCancel,
  pending,
  submitLabel,
}: {
  onCancel: () => void;
  pending: boolean;
  submitLabel?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex justify-end gap-2 pt-2">
      <Button variant="outline" onClick={onCancel}>
        {t('common.actions.cancel')}
      </Button>
      <Button type="submit" disabled={pending}>
        {pending ? t('common.actions.saving') : (submitLabel ?? t('common.actions.save'))}
      </Button>
    </div>
  );
}
