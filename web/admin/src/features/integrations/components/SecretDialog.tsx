import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
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
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@arenile/shared/ui/input-group';
import { Label } from '@arenile/shared/ui/label';
import { CopyIcon, TriangleAlertIcon } from 'lucide-react';
import { useId, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';

export interface SecretDialogProps {
  /** The secret; null closes the dialog. Kept only in the caller's state. */
  secret: string | null;
  title: string;
  description: ReactNode;
  /** Field label ("Chiave API", "Segreto di firma"). */
  label: string;
  /** Usage hint under the field (e.g. the Authorization header). */
  hint?: ReactNode;
  onClose: () => void;
}

/**
 * Shows a secret exactly once (API key, webhook signing secret) with a copy
 * button and the warning that it cannot be shown again. Closing the dialog
 * discards it: the caller clears its state in `onClose`.
 */
export function SecretDialog({
  secret,
  title,
  description,
  label,
  hint,
  onClose,
}: SecretDialogProps) {
  const { t } = useTranslation('integrations');
  const id = useId();
  const copy = async () => {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
      toast.success(t('secret.copied'));
    } catch {
      // Clipboard denied: the field stays selectable.
      toast.error(t('secret.copyFailed'));
    }
  };
  return (
    <Dialog open={secret !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <Alert>
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription>{t('secret.once')}</AlertDescription>
        </Alert>
        <div className="space-y-2">
          <Label htmlFor={id}>{label}</Label>
          <InputGroup>
            <InputGroupInput
              id={id}
              readOnly
              value={secret ?? ''}
              className="font-mono text-sm"
              onFocus={(e) => e.currentTarget.select()}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                aria-label={t('secret.copy')}
                onClick={() => void copy()}
              >
                <CopyIcon aria-hidden="true" />
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
        </div>
        <DialogFooter>
          <Button onClick={onClose}>{t('secret.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
