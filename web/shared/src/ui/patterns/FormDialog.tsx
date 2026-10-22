import type { FormEvent, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@arenile/shared/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@arenile/shared/ui/dialog';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { Spinner } from '@arenile/shared/ui/spinner';
import { cn } from '@arenile/shared/lib/utils';

export interface FormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** Visible description; without it the title doubles as the sr-only description. */
  description?: string;
  children: ReactNode;
  /** Wider dialog (sm:max-w-3xl) for multi-column forms. */
  wide?: boolean;
}

/** shadcn Dialog hosting a short create/edit form (scrolls above 90vh). */
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
        className={cn('max-h-[90vh] overflow-y-auto', wide ? 'sm:max-w-3xl' : 'sm:max-w-lg')}
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

export interface FormSheetProps extends Omit<FormDialogProps, 'wide'> {
  /** "lg" (sm:max-w-3xl) for long editors; default "md" (sm:max-w-2xl). */
  size?: 'md' | 'lg';
  /**
   * Pad and scroll the content (default). Pass false when the child is a
   * <SheetForm>, which scrolls its own body and keeps the footer in view.
   */
  padded?: boolean;
}

/** shadcn Sheet (right side panel) hosting a long editor. */
export function FormSheet({
  open,
  onOpenChange,
  title,
  description,
  children,
  size = 'md',
  padded = true,
}: FormSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        className={cn(
          'w-full gap-0',
          size === 'lg' ? 'sm:max-w-3xl' : 'sm:max-w-2xl',
          padded && 'overflow-y-auto',
        )}
      >
        <SheetHeader className="border-b">
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription className={cn(!description && 'sr-only')}>
            {description ?? title}
          </SheetDescription>
        </SheetHeader>
        {padded ? <div className="p-4">{children}</div> : children}
      </SheetContent>
    </Sheet>
  );
}

export interface SheetFormProps {
  onSubmit: (e?: FormEvent) => void;
  onCancel: () => void;
  pending: boolean;
  submitLabel?: string;
  /** Left side of the footer (e.g. a delete button). */
  footerStart?: ReactNode;
  children: ReactNode;
}

/** Form inside a <FormSheet padded={false}>: scrolling body, sticky footer with cancel / save. */
export function SheetForm({
  onSubmit,
  onCancel,
  pending,
  submitLabel,
  footerStart,
  children,
}: SheetFormProps) {
  const { t } = useTranslation();
  return (
    <form noValidate onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
      <div className="flex-1 space-y-8 overflow-y-auto p-4 sm:p-6">{children}</div>
      <SheetFooter className="flex-row items-center justify-between border-t bg-muted/30">
        <div>{footerStart}</div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onCancel}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={pending}>
            {pending && <Spinner />}
            {submitLabel ?? t('common.actions.save')}
          </Button>
        </div>
      </SheetFooter>
    </form>
  );
}

export interface FormActionsProps {
  onCancel: () => void;
  pending?: boolean;
  submitLabel?: string;
  /** Left side of the row (e.g. a delete button). */
  start?: ReactNode;
}

/** Cancel + submit row at the bottom of a dialog form. */
export function FormActions({ onCancel, pending, submitLabel, start }: FormActionsProps) {
  const { t } = useTranslation();
  return (
    <DialogFooter className={cn('border-t pt-4', start && 'sm:justify-between')}>
      {start && <div className="flex items-center">{start}</div>}
      <div className="flex flex-col-reverse gap-2 sm:flex-row">
        <Button variant="outline" onClick={onCancel}>
          {t('common.actions.cancel')}
        </Button>
        <Button type="submit" disabled={pending}>
          {pending && <Spinner />}
          {submitLabel ?? t('common.actions.save')}
        </Button>
      </div>
    </DialogFooter>
  );
}
