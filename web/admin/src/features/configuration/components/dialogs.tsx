import { Button } from '@arenile/shared/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@arenile/shared/ui/dialog';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { Spinner } from '@arenile/shared/ui/spinner';
import { cn } from '@arenile/shared/lib/utils';
import { ConfirmDialog } from '@arenile/shared/ui/patterns';
import { PencilIcon, Trash2Icon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useProblemText } from '../lib/problemText';

export interface FormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  wide?: boolean;
}

/** shadcn Dialog hosting a short create/edit form. */
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
        className={cn('max-h-[90vh] overflow-y-auto', wide && 'sm:max-w-3xl')}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className={cn(!description && 'sr-only')}>
            {description ?? title}
          </DialogDescription>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

/** shadcn Sheet (side panel) hosting a long editor, e.g. a catalog pack. */
export function FormSheet({ open, onOpenChange, title, description, children }: FormDialogProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-2xl">
        <SheetHeader className="border-b">
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription className={cn(!description && 'sr-only')}>
            {description ?? title}
          </SheetDescription>
        </SheetHeader>
        <div className="p-4">{children}</div>
      </SheetContent>
    </Sheet>
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

/** Delete icon button with the shared ConfirmDialog; failures are shown as a toast. */
export function ConfirmDelete({ label, message, onConfirm, disabled }: ConfirmDeleteProps) {
  const { t } = useTranslation();
  const problemText = useProblemText();
  return (
    <ConfirmDialog
      title={t('common.actions.confirmDelete')}
      description={message}
      confirmLabel={t('common.actions.delete')}
      destructive
      onConfirm={async () => {
        try {
          await onConfirm();
          toast.success(t('toast.deleted', { ns: 'configuration' }));
        } catch (err) {
          toast.error(problemText(err));
          throw err;
        }
      }}
      trigger={
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          title={label}
          disabled={disabled}
          className="text-muted-foreground hover:text-destructive"
        >
          <Trash2Icon aria-hidden="true" />
        </Button>
      }
    />
  );
}

/** Edit icon button + delete with confirmation, for table rows. */
export function RowActions({
  editLabel,
  deleteLabel,
  deleteMessage,
  onEdit,
  onDelete,
  children,
}: {
  editLabel: string;
  deleteLabel: string;
  deleteMessage: string;
  onEdit: () => void;
  onDelete: () => Promise<unknown>;
  children?: ReactNode;
}) {
  return (
    <div className="flex items-center justify-end gap-1">
      {children}
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onEdit}
        aria-label={editLabel}
        title={editLabel}
      >
        <PencilIcon aria-hidden="true" />
      </Button>
      <ConfirmDelete label={deleteLabel} message={deleteMessage} onConfirm={onDelete} />
    </div>
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
    <div className="flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end">
      <Button variant="outline" onClick={onCancel}>
        {t('common.actions.cancel')}
      </Button>
      <Button type="submit" disabled={pending}>
        {pending && <Spinner />}
        {submitLabel ?? t('common.actions.save')}
      </Button>
    </div>
  );
}
