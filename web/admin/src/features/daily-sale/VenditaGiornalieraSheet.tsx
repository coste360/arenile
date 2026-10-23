import { EmptyState } from '@arenile/shared/ui/patterns';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { ConstructionIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export interface VenditaGiornalieraSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Day of the sale (YYYY-MM-DD, establishment timezone). */
  date: string;
  /** Spot-first: the free spot tapped on the map. */
  spotId?: string;
  /** Zone-first: preselected zone (e.g. "Prima fila"). */
  zoneId?: string;
}

/**
 * Quick walk-in sale: where (spot or zone), slot, pack, extras
 * (QuantityStepper), price from the engine, cash or card, optional name.
 */
export function VenditaGiornalieraSheet({ open, onOpenChange }: VenditaGiornalieraSheetProps) {
  const { t } = useTranslation('dailySale');
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{t('sale.title')}</SheetTitle>
          <SheetDescription>{t('sale.description')}</SheetDescription>
        </SheetHeader>
        <div className="p-4">
          <EmptyState icon={ConstructionIcon} title={t('scaffold')} />
        </div>
      </SheetContent>
    </Sheet>
  );
}
