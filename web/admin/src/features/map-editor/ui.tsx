import {
  Alert,
  AlertDescription,
  Button,
  Kbd,
  KbdGroup,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@arenile/shared/ui';
import { CircleAlertIcon, EyeIcon, type LucideIcon } from 'lucide-react';
import type { ComponentProps, ComponentType, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { FormAlertState } from '../configuration/lib/formErrors';

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
