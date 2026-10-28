import type { Schemas } from '@arenile/shared/api';
import { FormSection } from '@arenile/shared/ui/patterns';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { BookingAgreementFields } from './BookingAgreementFields';
import {
  effectiveAgreementId,
  emptyAgreementValue,
  quoteFields,
  toCreateFields,
  type BookingAgreementValue,
} from './model';
import { useAgreementOptions } from './useAgreementOptions';

type BookingCreate = Schemas['BookingCreate'];

export interface NewBookingAgreement {
  value: BookingAgreementValue;
  /** Merged into the quote body: the agreement re-prices the selection live. */
  quoteFields: Partial<BookingCreate>;
  /** Merged into the create body: agreement, verification, partner and group. */
  createFields: Partial<BookingCreate>;
  /** "Convenzione e ospiti" section of the sheet. */
  section: ReactNode;
}

/**
 * State and section of the new booking sheet for the agreement (default from
 * the customer, spec 30.2.3), the requirement check, the partner and the
 * group (spec 30.3). The sheet merges the fields into its quote and create
 * bodies, so the price is re-computed as soon as the agreement changes.
 */
export function useNewBookingAgreement(
  establishmentId: string,
  customer: { agreement_id?: string } | null,
): NewBookingAgreement {
  const { t } = useTranslation('partners');
  const [value, setValue] = useState<BookingAgreementValue>(emptyAgreementValue);
  const agreementId = effectiveAgreementId(value, customer);
  const options = useAgreementOptions(establishmentId, agreementId, value.partnerId);
  return {
    value,
    quoteFields: quoteFields(value, customer),
    createFields: toCreateFields(value, customer, options.selected?.requirement),
    section: (
      <FormSection title={t('booking.section')} description={t('booking.sectionHint')}>
        <BookingAgreementFields
          value={value}
          onChange={setValue}
          options={options}
          customer={customer}
          verify
        />
      </FormSection>
    ),
  };
}
