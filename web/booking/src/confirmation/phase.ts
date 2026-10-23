import type { CheckoutState } from '../api/public';

export type ConfirmationPhase =
  | 'loading'
  | 'unknown'
  | 'waiting'
  | 'approval'
  | 'failed'
  | 'cancelled'
  | 'done'
  | 'expired'
  | 'rejected';

/**
 * What to show for a checkout state (pure): the page only reflects the
 * server, it never decides that a booking is confirmed (INV-19).
 */
export function confirmationPhase(state: CheckoutState | undefined): ConfirmationPhase {
  if (!state) return 'loading';
  switch (state.next_action) {
    case 'done':
      return 'done';
    case 'expired':
      return 'expired';
    case 'rejected':
      return 'rejected';
    case 'wait':
      return state.status === 'awaiting_approval' ? 'approval' : 'waiting';
    case 'pay':
    case 'details':
      return state.payment?.status === 'failed' ? 'failed' : 'cancelled';
  }
}
