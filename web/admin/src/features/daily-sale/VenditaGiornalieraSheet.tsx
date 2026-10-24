import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { useTranslation } from 'react-i18next';
import type { DailySaleResult } from './api';
import { DailySaleForm } from './DailySaleForm';
import type { SaleSlots } from './lib';

export interface VenditaGiornalieraSheetProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Day of the sale (YYYY-MM-DD, establishment timezone). */
  date: string;
  /** Spot-first: the free spot tapped on the map. */
  spotId?: string;
  /** Zone-first: preselected zone (e.g. "Prima fila"). */
  zoneId?: string;
  /** Slot shown on the map (default full day). */
  slots?: SaleSlots;
  onSold?: (result: DailySaleResult) => void;
}

/**
 * Quick walk-in sale from the day map ("Vendi giornata" on a free spot):
 * the same one-screen form as the "Vendita rapida" page, in a side sheet
 * with the total and the payment buttons kept at the bottom.
 */
export function VenditaGiornalieraSheet({
  establishmentId,
  open,
  onOpenChange,
  date,
  spotId,
  zoneId,
  slots,
  onSold,
}: VenditaGiornalieraSheetProps) {
  const { t } = useTranslation('dailySale');
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl">
        <SheetHeader className="border-b">
          <SheetTitle>{t('sale.sheetTitle')}</SheetTitle>
          <SheetDescription>{t('sale.sheetDescription')}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pt-4 sm:px-6">
          {open && (
            <DailySaleForm
              establishmentId={establishmentId}
              date={date}
              spotId={spotId}
              zoneId={zoneId}
              slots={slots}
              layout="sheet"
              onClose={() => onOpenChange(false)}
              onSold={onSold}
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
