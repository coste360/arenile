import { Button } from '@arenile/shared/ui/button';
import { SearchIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FindSpotSheet } from './FindSpotSheet';
import type { FindSpotDefaults } from './types';

export interface FindSpotLauncherProps {
  establishmentId: string;
  /** Read when the sheet opens (the dates and slot typed so far). */
  defaults: () => FindSpotDefaults;
  onBooked?: (bookingId: string) => void;
}

/** "Trova posto" button of the new booking (spec 30.6): opens the search in a sheet. */
export function FindSpotLauncher({ establishmentId, defaults, onBooked }: FindSpotLauncherProps) {
  const { t } = useTranslation('findspot');
  const [open, setOpen] = useState<FindSpotDefaults | null>(null);
  return (
    <>
      <Button variant="outline" onClick={() => setOpen(defaults())}>
        <SearchIcon aria-hidden="true" />
        {t('open')}
      </Button>
      {open && (
        <FindSpotSheet
          establishmentId={establishmentId}
          open
          onOpenChange={(v) => !v && setOpen(null)}
          defaults={open}
          onBooked={(id) => {
            setOpen(null);
            onBooked?.(id);
          }}
        />
      )}
    </>
  );
}
