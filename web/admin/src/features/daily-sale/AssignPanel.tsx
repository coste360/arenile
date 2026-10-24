import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { MapPinIcon, SparklesIcon, XIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { localize } from '../configuration/lib/i18nText';
import type { Ticket } from './api';

export interface AssignPanelProps {
  tickets: readonly Ticket[];
  /** Ticket picked for assignment; the next free spot tapped receives it. */
  selectedTicketId?: string;
  onSelectTicket: (ticketId: string | undefined) => void;
  onAutoAssign: () => void;
  autoAssigning?: boolean;
  /** Assignments are a write: read-only users see the list only. */
  canWrite: boolean;
}

/**
 * "Da assegnare" panel of the day map (desktop and phone): the zone-first
 * tickets (#037 ...) as large toggles; pick one, then tap a free spot of its
 * zone on the map. "Assegna automaticamente" fills free spots in order.
 */
export function AssignPanel({
  tickets,
  selectedTicketId,
  onSelectTicket,
  onAutoAssign,
  autoAssigning,
  canWrite,
}: AssignPanelProps) {
  const { t, i18n } = useTranslation('dailySale');
  const locale = i18n.language || 'it';
  const selected = tickets.find((x) => x.id === selectedTicketId);
  return (
    <section
      aria-labelledby="assign-panel-title"
      className="space-y-3 rounded-lg border border-warning bg-warning/10 p-3"
      data-testid="assign-panel"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="assign-panel-title" className="flex items-center gap-2 text-sm font-semibold">
          <MapPinIcon aria-hidden="true" className="size-4" />
          {t('assign.title')}
          <Badge variant="secondary">{tickets.length}</Badge>
        </h2>
        {canWrite && (
          <Button size="sm" variant="outline" onClick={onAutoAssign} disabled={autoAssigning}>
            {autoAssigning ? <Spinner /> : <SparklesIcon aria-hidden="true" />}
            {t('assign.auto')}
          </Button>
        )}
      </div>
      <ul
        className="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap"
        aria-label={t('assign.listLabel')}
      >
        {tickets.map((x) => {
          const zone = localize(x.zone_name, locale, 'it');
          const pressed = x.id === selectedTicketId;
          return (
            <li key={x.id}>
              <Button
                variant={pressed ? 'default' : 'outline'}
                className="h-auto min-h-11 w-full flex-col items-start gap-0 py-1.5 text-left whitespace-normal sm:w-auto"
                aria-pressed={pressed}
                aria-label={t('assign.ticketAria', {
                  number: x.number_label,
                  zone,
                  slots: t(`slots.${x.slots}`),
                })}
                disabled={!canWrite}
                onClick={() => onSelectTicket(pressed ? undefined : x.id)}
              >
                <span className="font-semibold tabular-nums">
                  {x.number_label}
                  {x.holder_name ? ` · ${x.holder_name}` : ''}
                </span>
                <span className="text-xs font-normal opacity-80">
                  {zone} · {t(`slots.${x.slots}`)}
                </span>
              </Button>
            </li>
          );
        })}
      </ul>
      <p role="status" aria-live="polite" className="text-sm">
        {selected ? (
          <span className="flex flex-wrap items-center gap-2">
            {t('assign.pick', {
              number: selected.number_label,
              zone: localize(selected.zone_name, locale, 'it'),
            })}
            <Button size="sm" variant="ghost" onClick={() => onSelectTicket(undefined)}>
              <XIcon aria-hidden="true" />
              {t('common.actions.cancel', { ns: 'translation' })}
            </Button>
          </span>
        ) : canWrite ? (
          <span className="text-muted-foreground">{t('assign.description')}</span>
        ) : null}
      </p>
    </section>
  );
}
