import { Button } from '@arenile/shared/ui/button';
import { ButtonGroup } from '@arenile/shared/ui/button-group';
import { DatePicker, OptionSelect } from '@arenile/shared/ui/patterns';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { addDays } from '../lib/dates';
import type { SlotChoice } from '../lib/status';

const SLOTS: readonly SlotChoice[] = ['FULL', 'AM', 'PM'];

export interface DayToolbarProps {
  date: string;
  slot: SlotChoice;
  today: string;
  onChange: (next: { date?: string; slot?: SlotChoice }) => void;
  maps?: readonly { id: string; name: string }[];
  mapId?: string;
  onMapChange?: (mapId: string) => void;
}

/**
 * Day selector always visible on the operational pages (spec 11.3): previous
 * and next day, "oggi", a date picker, the slot toggle and, with more maps,
 * the map selector.
 */
export function DayToolbar({
  date,
  slot,
  today,
  onChange,
  maps,
  mapId,
  onMapChange,
}: DayToolbarProps) {
  const { t } = useTranslation('operations');
  return (
    <div
      role="toolbar"
      aria-label={t('toolbar.label')}
      className="flex flex-wrap items-center gap-2"
    >
      <ButtonGroup>
        <Button
          variant="outline"
          size="icon"
          aria-label={t('toolbar.previousDay')}
          onClick={() => onChange({ date: addDays(date, -1) })}
        >
          <ChevronLeftIcon aria-hidden="true" />
        </Button>
        <Button
          variant="outline"
          disabled={date === today}
          onClick={() => onChange({ date: today })}
        >
          {t('toolbar.today')}
        </Button>
        <Button
          variant="outline"
          size="icon"
          aria-label={t('toolbar.nextDay')}
          onClick={() => onChange({ date: addDays(date, 1) })}
        >
          <ChevronRightIcon aria-hidden="true" />
        </Button>
      </ButtonGroup>
      <DatePicker
        value={date}
        onChange={(v) => v && onChange({ date: v })}
        aria-label={t('toolbar.date')}
        className="sm:w-44"
      />
      <ToggleGroup
        type="single"
        variant="outline"
        value={slot}
        onValueChange={(v) => v && onChange({ slot: v as SlotChoice })}
        aria-label={t('toolbar.slot')}
      >
        {SLOTS.map((s) => (
          <ToggleGroupItem key={s} value={s} className="px-3">
            {t(`slot.${s}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {maps && maps.length > 1 && onMapChange && (
        <OptionSelect
          value={mapId ?? ''}
          onChange={onMapChange}
          options={maps.map((m) => ({ value: m.id, label: m.name }))}
          aria-label={t('toolbar.map')}
          className="sm:w-48"
        />
      )}
    </div>
  );
}
