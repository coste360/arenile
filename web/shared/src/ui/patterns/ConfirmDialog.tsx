import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@arenile/shared/ui/alert-dialog';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CircleAlertIcon } from 'lucide-react';

export interface ConfirmDialogProps {
  title: ReactNode;
  description?: ReactNode;
  /** Label of the confirm button (default "Conferma"). */
  confirmLabel?: ReactNode;
  cancelLabel?: ReactNode;
  /** Red confirm button for irreversible actions. */
  destructive?: boolean;
  /**
   * Called on confirm. When it returns a promise the dialog stays open with a
   * spinner until it settles, and closes only on success.
   */
  onConfirm: () => void | Promise<unknown>;
  /** Element opening the dialog (rendered with asChild). Omit when controlled. */
  trigger?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /**
   * Text of a rejected `onConfirm`, shown inside the dialog (which stays open
   * so the action can be retried). Without it the caller reports the error.
   */
  errorText?: (error: unknown) => string;
}

/** Confirmation for destructive or important actions (shadcn AlertDialog). */
export function ConfirmDialog({
  title,
  description,
  confirmLabel,
  cancelLabel,
  destructive,
  onConfirm,
  trigger,
  open: openProp,
  onOpenChange,
  errorText,
}: ConfirmDialogProps) {
  const { t } = useTranslation();
  const [openState, setOpenState] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const open = openProp ?? openState;
  const setOpen = (v: boolean) => {
    if (openProp === undefined) setOpenState(v);
    if (v) setError(null);
    onOpenChange?.(v);
  };

  const confirm = async (e: React.MouseEvent) => {
    e.preventDefault();
    const result = onConfirm();
    if (!(result instanceof Promise)) {
      setOpen(false);
      return;
    }
    setPending(true);
    setError(null);
    try {
      await result;
      setOpen(false);
    } catch (err) {
      // Keep the dialog open: show the error here when asked, else the caller reports it.
      if (errorText) setError(errorText(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <AlertDialog open={open} onOpenChange={(v) => !pending && setOpen(v)}>
      {trigger && <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>}
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        {error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>
            {cancelLabel ?? t('ui.confirm.cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={pending}
            onClick={(e) => void confirm(e)}
            variant={destructive ? 'destructive' : 'default'}
          >
            {pending && <Spinner />}
            {confirmLabel ?? t('ui.confirm.confirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
