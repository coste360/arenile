import type { Schemas } from '@arenile/shared/api';
import type { CheckoutCustomerInput, CheckoutState } from '../api/public';

export interface WhoStepProps {
  state: CheckoutState;
  terms?: Schemas['PublicTerms'];
  requirePhone: boolean;
  /** PATCH the customer details, consents and terms version. */
  onSubmit: (input: CheckoutCustomerInput) => Promise<void>;
  /** POST pay; redirect / embedded / none handled by the caller. */
  onPay: () => Promise<void>;
}

/**
 * "Chi e pagamento" step (spec 12.3 passo 4): first and last name, email,
 * phone, no account; versioned terms with acceptance and the separate,
 * optional marketing consent; visible hold countdown (who/HoldCountdown);
 * pay or confirm. Payment redirects at top level (`target=_top` from the
 * iframe through the embed protocol `arenile:navigate`, new tab if the host
 * blocks it). Owner: booking app builder.
 */
export function WhoStep(_props: WhoStepProps) {
  return null;
}
