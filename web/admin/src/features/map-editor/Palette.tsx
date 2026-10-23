import type { KitAsset } from '@arenile/shared/map';
import {
  Button,
  ButtonGroup,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@arenile/shared/ui';
import { ChevronDownIcon, ShapesIcon, TypeIcon, UmbrellaIcon, type LucideIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { KitPicker, type KitFamily } from './KitPicker';

function PickerButton({
  family,
  icon: Icon,
  text,
  hint,
  disabled,
  children,
}: {
  family: KitFamily;
  icon: LucideIcon;
  text: string;
  hint: string;
  disabled?: boolean;
  children: (close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              disabled={disabled}
              data-palette={family}
              aria-haspopup="dialog"
            >
              <Icon aria-hidden="true" />
              {text}
              <ChevronDownIcon aria-hidden="true" className="opacity-60" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{hint}</TooltipContent>
      </Tooltip>
      <PopoverContent align="start" className="w-[min(28rem,calc(100vw-2rem))] p-0">
        {open && children(() => setOpen(false))}
      </PopoverContent>
    </Popover>
  );
}

/**
 * Palette of the elements that can be added (spec 11.2): two searchable,
 * categorised pickers of kit illustrations, one for bookable spots and one
 * for decorations (plus free text labels).
 */
export function Palette({
  canAddSpots,
  onAddSpot,
  onAddDecoration,
  onAddText,
}: {
  canAddSpots: boolean;
  onAddSpot: (asset: KitAsset) => void;
  onAddDecoration: (asset: KitAsset) => void;
  onAddText: () => void;
}) {
  const { t } = useTranslation('mapEditor');
  return (
    <ButtonGroup aria-label={t('palette.label')}>
      <PickerButton
        family="spots"
        icon={UmbrellaIcon}
        text={t('palette.spots')}
        hint={t('palette.spotsHint')}
        disabled={!canAddSpots}
      >
        {(close) => (
          <KitPicker
            family="spots"
            label={t('palette.spots')}
            onPick={(a) => {
              onAddSpot(a);
              close();
            }}
          />
        )}
      </PickerButton>
      <PickerButton
        family="decorations"
        icon={ShapesIcon}
        text={t('palette.decorations')}
        hint={t('palette.decorationsHint')}
      >
        {(close) => (
          <KitPicker
            family="decorations"
            label={t('palette.decorations')}
            extras={[{ value: 'text', label: t('decorations.text'), icon: TypeIcon }]}
            onPickExtra={() => {
              onAddText();
              close();
            }}
            onPick={(a) => {
              onAddDecoration(a);
              close();
            }}
          />
        )}
      </PickerButton>
    </ButtonGroup>
  );
}
