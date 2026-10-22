import { ApiProblem } from '@arenile/shared/api';
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Kbd,
  KbdGroup,
  Skeleton,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@arenile/shared/ui';
import { CircleAlertIcon, EyeIcon, type LucideIcon } from 'lucide-react';
import type { ComponentProps, ComponentType, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../../lib/problem';
import type { FormAlertState } from '../configuration/lib/formErrors';

/** Loading skeleton / error with retry for a query. */
export function QueryState({
  isLoading,
  error,
  onRetry,
  skeleton = 'list',
}: {
  isLoading: boolean;
  error: unknown;
  onRetry?: () => void;
  skeleton?: 'list' | 'editor';
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <Alert variant="destructive">
        <CircleAlertIcon aria-hidden="true" />
        <AlertTitle>
          {t(problemMessageKey(error instanceof ApiProblem ? error : undefined))}
        </AlertTitle>
        {onRetry && (
          <AlertDescription>
            <Button size="sm" variant="outline" onClick={onRetry} className="mt-1">
              {t('common.retry')}
            </Button>
          </AlertDescription>
        )}
      </Alert>
    );
  }
  if (!isLoading) return null;
  return (
    <div role="status" aria-label={t('common.loading')} className="space-y-3">
      {skeleton === 'editor' ? (
        <>
          <Skeleton className="h-11 w-full" />
          <Skeleton className="h-[60vh] w-full" />
        </>
      ) : (
        <>
          <Skeleton className="h-9 w-64" />
          <Skeleton className="h-40 w-full" />
        </>
      )}
    </div>
  );
}

/** Banner for roles that can only view the maps. */
export function ReadOnlyNotice() {
  const { t } = useTranslation('mapEditor');
  return (
    <Alert role="note">
      <EyeIcon aria-hidden="true" />
      <AlertDescription>{t('editor.readOnly')}</AlertDescription>
    </Alert>
  );
}

/** Non-field errors of a form (problem+json mapped by problemToFormState). */
export function FormAlert({ state }: { state: FormAlertState | null }) {
  const { t } = useTranslation();
  if (!state) return null;
  return (
    <Alert variant="destructive">
      <CircleAlertIcon aria-hidden="true" />
      <AlertDescription>
        {state.kind === 'conflict' && <p>{t('common.errors.versionConflict')}</p>}
        {state.kind === 'message' && <p>{t(state.key)}</p>}
        {state.kind === 'server' && (
          <ul className="list-inside list-disc">
            {state.messages.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  );
}

const IS_MAC =
  typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

/** Keyboard shortcut as Kbd keys; "Mod" is Cmd on Apple devices, Ctrl elsewhere. */
export function Shortcut({ keys }: { keys: string[] }) {
  return (
    <KbdGroup>
      {keys.map((k) => (
        <Kbd key={k}>{k === 'Mod' ? (IS_MAC ? '⌘' : 'Ctrl') : k === 'Shift' ? '⇧' : k}</Kbd>
      ))}
    </KbdGroup>
  );
}

export interface ToolButtonProps extends Omit<ComponentProps<typeof Button>, 'children'> {
  icon: LucideIcon | ComponentType<{ 'aria-hidden'?: 'true' }>;
  /** Accessible name and tooltip text. */
  label: string;
  /** Visible text next to the icon (default: icon only). */
  text?: ReactNode;
  /** Shortcut shown in the tooltip, e.g. ['Mod', 'Z']. */
  shortcut?: string[];
}

/** Toolbar button: lucide icon, tooltip with the label and its keyboard shortcut. */
export function ToolButton({
  icon: Icon,
  label,
  text,
  shortcut,
  variant = 'ghost',
  size,
  ...props
}: ToolButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant={variant}
          size={size ?? (text ? 'sm' : 'icon-sm')}
          aria-label={text ? undefined : label}
          {...props}
        >
          <Icon aria-hidden="true" />
          {text}
        </Button>
      </TooltipTrigger>
      <TooltipContent className="flex items-center gap-2">
        {label}
        {shortcut && <Shortcut keys={shortcut} />}
      </TooltipContent>
    </Tooltip>
  );
}

/** Modal dialog hosting a form (shadcn Dialog). */
export function FormDialog({
  open,
  onOpenChange,
  title,
  description,
  wide,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  wide?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        closeLabel={t('common.close')}
        className={wide ? 'max-h-[90vh] overflow-y-auto sm:max-w-3xl' : 'sm:max-w-lg'}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : (
            <DialogDescription className="sr-only">{title}</DialogDescription>
          )}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

/** Cancel + submit footer of a dialog form. */
export function FormActions({
  onCancel,
  pending,
  submitLabel,
}: {
  onCancel: () => void;
  pending?: boolean;
  submitLabel?: string;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter>
      <Button variant="outline" onClick={onCancel}>
        {t('common.actions.cancel')}
      </Button>
      <Button type="submit" disabled={pending}>
        {pending && <Spinner />}
        {submitLabel ?? t('common.actions.save')}
      </Button>
    </DialogFooter>
  );
}
