import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Kbd } from '@arenile/shared/ui/kbd';
import { BanIcon, ListChecksIcon, UsersIcon, XIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { SelectionActions } from '../lib/multiSelect';

export interface SelectionBarProps {
  /** Labels of the selected spots, in selection order. */
  labels: readonly string[];
  actions: SelectionActions;
  onGroupBooking: () => void;
  onBlock: () => void;
  onClear: () => void;
  onExit: () => void;
}

/**
 * Bar of the multi-selection mode of the operational map (ADR 0019 C): how
 * to select (tap, Shift for a range, drag for a rectangle), the live count
 * and the actions on the selection. The count is a polite live region.
 */
export function SelectionBar({
  labels,
  actions,
  onGroupBooking,
  onBlock,
  onClear,
  onExit,
}: SelectionBarProps) {
  const { t } = useTranslation('operations');
  const count = labels.length;
  return (
    <Alert data-testid="selection-bar">
      <ListChecksIcon aria-hidden="true" />
      <AlertTitle role="status" aria-live="polite">
        {count > 0 ? t('select.count', { count }) : t('select.none')}
      </AlertTitle>
      <AlertDescription className="space-y-3">
        <p>
          {t('select.hint')}{' '}
          <span className="hidden sm:inline">
            {t('select.hintKeys')} <Kbd>Shift</Kbd>
          </span>
        </p>
        {count > 0 && (
          <p className="text-xs break-words text-muted-foreground">{labels.join(', ')}</p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={onGroupBooking} disabled={!actions.groupBooking}>
            <UsersIcon aria-hidden="true" />
            {t('select.groupBooking')}
          </Button>
          <Button size="sm" variant="outline" onClick={onBlock} disabled={!actions.block}>
            <BanIcon aria-hidden="true" />
            {t('select.block')}
          </Button>
          {count > 0 && (
            <Button size="sm" variant="ghost" onClick={onClear}>
              {t('select.clear')}
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={onExit}>
            <XIcon aria-hidden="true" />
            {t('select.exit')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}
