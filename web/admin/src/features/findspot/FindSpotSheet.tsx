import { FormSheet } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';
import { FindSpotPanel, type FindSpotPanelProps } from './FindSpotPanel';

export interface FindSpotSheetProps extends FindSpotPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** "Trova posto" in a side sheet, from the map toolbar and the new booking (spec 30.6). */
export function FindSpotSheet({ open, onOpenChange, ...panel }: FindSpotSheetProps) {
  const { t } = useTranslation('findspot');
  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t('title')}
      description={t('description')}
      size="lg"
    >
      {open && <FindSpotPanel {...panel} />}
    </FormSheet>
  );
}
