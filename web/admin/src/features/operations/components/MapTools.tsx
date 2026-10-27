import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { ListOrderedIcon, MapIcon, PrinterIcon, SearchIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { MapPaper } from './DayMapSheet';
import { UndoLastMoveButton } from './UndoLastMoveButton';

export interface MapToolsProps {
  establishmentId: string;
  /** "Trova posto" (bookings.write). */
  onFindSpot?: () => void;
  onPrintMap: (paper: MapPaper) => void;
  onLifeguardList: () => void;
  /** Show "Annulla ultimo spostamento" (bookings.write). */
  canUndo: boolean;
}

/**
 * Cash-desk tools above the operational map (spec 30.6, 30.7, 30.10):
 * "Trova posto", undo of the last move and printing (map of the day in A4
 * or A3 landscape, list for the lifeguards).
 */
export function MapTools({
  establishmentId,
  onFindSpot,
  onPrintMap,
  onLifeguardList,
  canUndo,
}: MapToolsProps) {
  const { t } = useTranslation('operations');
  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="map-tools">
      {onFindSpot && (
        <Button variant="outline" onClick={onFindSpot}>
          <SearchIcon aria-hidden="true" />
          {t('tools.findSpot')}
        </Button>
      )}
      <UndoLastMoveButton establishmentId={establishmentId} enabled={canUndo} />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline">
            <PrinterIcon aria-hidden="true" />
            {t('tools.print')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>{t('tools.printMap')}</DropdownMenuLabel>
          <DropdownMenuItem onSelect={() => onPrintMap('a4')}>
            <MapIcon aria-hidden="true" />
            {t('tools.printA4')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onPrintMap('a3')}>
            <MapIcon aria-hidden="true" />
            {t('tools.printA3')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={onLifeguardList}>
            <ListOrderedIcon aria-hidden="true" />
            {t('tools.lifeguard')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
