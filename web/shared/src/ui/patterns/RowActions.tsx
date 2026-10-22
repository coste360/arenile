import { PencilIcon, Trash2Icon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useProblemText } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { ConfirmDialog } from './ConfirmDialog';

/** Pencil icon button opening an editor. */
export function EditButton({
  label,
  onClick,
  disabled,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      title={label}
      onClick={onClick}
      disabled={disabled}
    >
      <PencilIcon aria-hidden="true" />
    </Button>
  );
}

export interface DeleteButtonProps {
  /** Accessible name of the trigger, e.g. "Elimina la stagione 2027". */
  label: string;
  /** Text of the confirmation. */
  message: string;
  onConfirm: () => Promise<unknown>;
  /** Show "Elimina" next to the icon (e.g. in a form footer). */
  withText?: boolean;
  disabled?: boolean;
}

/**
 * The one delete style: grey icon, red on hover, red only inside the
 * confirmation. A failed delete keeps the dialog open and shows the server
 * detail inside it; success shows an "Eliminato" toast.
 */
export function DeleteButton({ label, message, onConfirm, withText, disabled }: DeleteButtonProps) {
  const { t } = useTranslation();
  const problemText = useProblemText();
  return (
    <ConfirmDialog
      title={t('common.actions.confirmDelete')}
      description={message}
      confirmLabel={t('common.actions.delete')}
      destructive
      errorText={problemText}
      onConfirm={async () => {
        await onConfirm();
        toast.success(t('ui.deleted'));
      }}
      trigger={
        <Button
          variant="ghost"
          size={withText ? 'sm' : 'icon-sm'}
          aria-label={label}
          title={withText ? undefined : label}
          disabled={disabled}
          className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
        >
          <Trash2Icon aria-hidden="true" />
          {withText && t('common.actions.delete')}
        </Button>
      }
    />
  );
}

export interface RowActionsProps {
  /** Extra actions rendered first (links, toggles). */
  children?: ReactNode;
  editLabel?: string;
  onEdit?: () => void;
  deleteLabel?: string;
  deleteMessage?: string;
  onDelete?: () => Promise<unknown>;
  disabled?: boolean;
}

/** Right-aligned icon actions of a table row: custom children, then edit, then delete with confirmation. */
export function RowActions({
  children,
  editLabel,
  onEdit,
  deleteLabel,
  deleteMessage,
  onDelete,
  disabled,
}: RowActionsProps) {
  return (
    <div className="flex items-center justify-end gap-0.5">
      {children}
      {onEdit && editLabel && <EditButton label={editLabel} onClick={onEdit} disabled={disabled} />}
      {onDelete && deleteLabel && (
        <DeleteButton
          label={deleteLabel}
          message={deleteMessage ?? deleteLabel}
          onConfirm={onDelete}
          disabled={disabled}
        />
      )}
    </div>
  );
}
