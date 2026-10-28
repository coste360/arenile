import type { Customer } from '../api';
import { customerValues, toCustomerInput } from '../customer.schema';

/** Full replacement of the customer with the new default agreement and categories. */
export function withAgreement(
  c: Customer,
  agreementId: string,
  categoryIds: string[],
): ReturnType<typeof toCustomerInput> {
  const input = toCustomerInput(customerValues(c), c);
  delete input.agreement_id;
  return {
    ...input,
    ...(agreementId ? { agreement_id: agreementId } : {}),
    category_ids: categoryIds,
  };
}
