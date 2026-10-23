import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { useTranslation } from 'react-i18next';

export interface NewBookingSheetProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Prefill from the map or the planning (selected spots and dates). */
  defaults?: {
    spotIds?: string[];
    startsOn?: string;
    endsOn?: string;
    slots?: 'AM' | 'PM' | 'FULL';
    holdId?: string;
  };
}

/**
 * Side sheet to create a booking from the panel (quote preview, then
 * POST /bookings). Placeholder of the final component (M3 scaffolding,
 * owner: the bookings builder); operations and planning open it with
 * `defaults` from their selection.
 */
export function NewBookingSheet({ open, onOpenChange }: NewBookingSheetProps) {
  const { t } = useTranslation('bookings');
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{t('new.title')}</SheetTitle>
          <SheetDescription>{t('new.description')}</SheetDescription>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  );
}
