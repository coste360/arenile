import { DatePicker } from '@arenile/shared/ui/patterns';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { moveFromChoices, type MoveFromChoice, type StayWindow } from '../lib/moveFrom';
import { addDays } from '../lib/dates';

export interface MoveFromValue {
  choice: MoveFromChoice;
  date?: string;
}

export interface MoveFromFieldProps {
  stay: StayWindow;
  today: string;
  value: MoveFromValue;
  onChange: (value: MoveFromValue) => void;
  disabled?: boolean;
}

/**
 * "Da quando" of a move (spec 30.7): the whole stay, today, tomorrow or a
 * chosen date inside the stay. A future date schedules the move ("dal 15
 * agosto passa ad A7"): the server moves the future allocations at once.
 */
export function MoveFromField({ stay, today, value, onChange, disabled }: MoveFromFieldProps) {
  const { t } = useTranslation('operations');
  const id = useId();
  const choices = moveFromChoices(stay, today);
  if (choices.length <= 1 && choices[0] !== 'date') return null;
  const min = addDays(stay.starts_on, 1) > today ? addDays(stay.starts_on, 1) : today;
  return (
    <div className="space-y-2">
      <p id={id} className="text-sm font-medium">
        {t('moveFrom.label')}
      </p>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        spacing={1}
        aria-labelledby={id}
        value={value.choice}
        disabled={disabled}
        onValueChange={(v) => {
          if (!v) return;
          const choice = v as MoveFromChoice;
          onChange({ choice, date: choice === 'date' ? (value.date ?? min) : undefined });
        }}
        className="flex-wrap"
      >
        {choices.map((c) => (
          <ToggleGroupItem key={c} value={c}>
            {t(`moveFrom.${c}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {value.choice === 'date' && (
        <DatePicker
          aria-label={t('moveFrom.dateLabel')}
          value={value.date}
          min={min}
          max={stay.ends_on}
          disabled={disabled}
          onChange={(d) => onChange({ choice: 'date', date: d })}
        />
      )}
    </div>
  );
}
