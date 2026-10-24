import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { localize } from '../configuration/lib/i18nText';
import type { OperationalSpot, SlotChoice } from '../operations/lib/status';
import {
  useAssignTicket,
  useAutoAssignTickets,
  useUnassignedTickets,
  type Ticket,
} from './api';
import { AssignPanel } from './AssignPanel';
import { assignmentTargets, autoAssignSummary } from './lib';
import { useSaleProblemText } from './useSaleProblemText';

export interface TicketAssignment {
  /** The panel to render above the map (null without tickets to place). */
  panel: ReactNode;
  /** True while a ticket is picked: taps on the map assign it. */
  active: boolean;
  /** The picked ticket (the map scrolls to and zooms on its zone). */
  picked?: Ticket;
  /** Spots the picked ticket can take (free in its zone, or its own provisional spot). */
  highlightIds: string[];
  /** Handles a tap on a spot while assigning; returns false when not assigning. */
  handleSpot: (spotId: string) => boolean;
}

/**
 * Tap-ticket-then-spot assignment of the day map: owns the "Da assegnare"
 * panel, the picked ticket and the atomic assignment (409 when the spot was
 * taken in the meantime, shown as a toast; the map refreshes either way).
 */
export function useTicketAssignment({
  establishmentId,
  date,
  slot,
  spots,
  canWrite,
}: {
  establishmentId: string;
  date: string;
  slot: SlotChoice;
  spots: readonly OperationalSpot[];
  canWrite: boolean;
}): TicketAssignment {
  const { t, i18n } = useTranslation('dailySale');
  const problemText = useSaleProblemText();
  const unassigned = useUnassignedTickets(establishmentId, date);
  const assign = useAssignTicket(establishmentId);
  const auto = useAutoAssignTickets(establishmentId);
  const [pickedId, setPickedId] = useState<string | undefined>();
  const tickets = useMemo(() => unassigned.data?.items ?? [], [unassigned.data]);
  const picked = tickets.find((x) => x.id === pickedId);
  const byId = useMemo(() => new Map(spots.map((s) => [s.spot_id, s])), [spots]);

  // Spots the picked ticket can take: the free ones of its zone and the
  // spot its own booking holds provisionally (seat zones: the sale parked
  // the ticket on the last free spot, so a full zone still has this target).
  const targets = useMemo(
    () => (picked ? assignmentTargets(spots, picked, slot) : []),
    [picked, spots, slot],
  );
  const highlightIds = useMemo(() => targets.map((s) => s.spot_id), [targets]);

  const handleSpot = (spotId: string): boolean => {
    if (!picked) return false;
    const spot = byId.get(spotId);
    const zone = localize(picked.zone_name, i18n.language || 'it', 'it');
    if (!spot || !targets.some((s) => s.spot_id === spotId)) {
      toast.error(spot && spot.zone_id !== picked.zone_id ? t('assign.wrongZone', { zone }) : t('assign.notFree'));
      return true;
    }
    const ticket = picked;
    setPickedId(undefined);
    assign.mutate(
      { ticketId: ticket.id, spotId, version: ticket.version },
      {
        onSuccess: () =>
          toast.success(t('assign.done', { number: ticket.number_label, label: spot.label })),
        onError: (e) => toast.error(problemText(e)),
      },
    );
    return true;
  };

  const runAuto = () =>
    auto.mutate(
      { date },
      {
        onSuccess: (r) => {
          const s = autoAssignSummary(r);
          setPickedId(undefined);
          if (s.left > 0)
            toast.warning(t('assign.autoPartial', { count: s.assigned, left: s.left }));
          else toast.success(t('assign.autoDone', { count: s.assigned }));
        },
        onError: (e) => toast.error(problemText(e)),
      },
    );

  const panel =
    tickets.length > 0 ? (
      <AssignPanel
        tickets={tickets}
        selectedTicketId={picked?.id}
        onSelectTicket={setPickedId}
        onAutoAssign={runAuto}
        autoAssigning={auto.isPending}
        canWrite={canWrite}
        targets={targets.map((s) => ({ id: s.spot_id, label: s.label }))}
        onPickSpot={handleSpot}
      />
    ) : null;

  return { panel, active: !!picked, picked, highlightIds, handleSpot };
}
