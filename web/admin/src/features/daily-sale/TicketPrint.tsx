import { formatCents } from '@arenile/shared/lib/money';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { cn } from '@arenile/shared/lib/utils';
import { useTranslation } from 'react-i18next';
import type { TicketPrintData } from './api';

export interface TicketPrintProps {
  data: TicketPrintData;
  className?: string;
}

/**
 * Printable ticket (thermal 80/58 mm, A4, A5), styled only with Tailwind
 * (print: utilities hide everything else). The QR carries data.qr_payload.
 */
export function TicketPrint({ data, className }: TicketPrintProps) {
  const { t } = useTranslation('dailySale');
  const { ticket } = data;
  const narrow = data.format === 'thermal_58' || data.format === 'thermal_80';
  return (
    <article
      aria-label={t('print.label', { number: ticket.number_label })}
      className={cn(
        'mx-auto bg-background text-foreground print:bg-transparent print:text-foreground',
        narrow ? 'max-w-xs font-mono text-xs' : 'max-w-2xl text-sm',
        className,
      )}
    >
      <header className="text-center">
        <p className="font-semibold">{data.establishment.name}</p>
        {data.reprint && <p className="uppercase">{t('print.reprint')}</p>}
      </header>
      <p className="mt-2 text-center text-2xl font-bold tabular-nums">{ticket.number_label}</p>
      <p className="text-center">{formatIsoDate(ticket.ticket_date)}</p>
      <p className="mt-2">{ticket.spot_label ?? t('print.unassigned')}</p>
      <p className="mt-2 text-right font-semibold tabular-nums">
        {formatCents(ticket.total_amount)}
      </p>
    </article>
  );
}
