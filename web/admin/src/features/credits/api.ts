/**
 * Server state of the credits feature (M5B: customer credit ledger on the customer detail (spec 13.4)): types from the
 * generated schema. The credits frontend builder adds the query key factory
 * (establishmentKeys.area(id, 'credits')) and the TanStack Query hooks here.
 */
import type { Schemas } from '@arenile/shared/api';

export type CustomerCredit = Schemas['CustomerCredit'];
export type CustomerCreditLedger = Schemas['CustomerCreditLedger'];
export type CustomerCreditCreate = Schemas['CustomerCreditCreate'];
export type CustomerCreditExpiring = Schemas['CustomerCreditExpiring'];
