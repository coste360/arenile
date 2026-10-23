import { SectionCard } from '@arenile/shared/ui/patterns';
import { useTranslation } from 'react-i18next';

export interface AssignPanelProps {
  establishmentId: string;
  /** Day shown on the map (YYYY-MM-DD). */
  date: string;
  /** Ticket picked for assignment; the next free spot tapped receives it. */
  selectedTicketId?: string;
  onSelectTicket?: (ticketId: string | undefined) => void;
}

/**
 * "Da assegnare" panel of the day map (desktop and phone): lists the
 * zone-first tickets (#037 ...), picks one, then a free spot of its zone;
 * "Assegna automaticamente" fills free spots in order.
 */
export function AssignPanel(_props: AssignPanelProps) {
  const { t } = useTranslation('dailySale');
  return (
    <SectionCard title={t('assign.title')} description={t('assign.description')}>
      <p className="text-sm text-muted-foreground">{t('scaffold')}</p>
    </SectionCard>
  );
}
