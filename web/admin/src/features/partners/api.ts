/**
 * Server state of "Partner e convenzioni" (spec 30.2, ADR 0028): partners and
 * customer categories (configuration resources, invalidated with the rest of
 * the configuration, agreements included), the partner account (statement and
 * recorded payments), requirement verifications and agreement versions/quota.
 * Types come from the generated contract; money is integer cents.
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey, createResource } from '../configuration/api/resource';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';

export type Partner = Schemas['Partner'];
export type PartnerInput = Schemas['PartnerInput'];
export type PartnerKind = Schemas['PartnerKind'];
export type AccommodationType = Schemas['AccommodationType'];
export type PartnerRef = Schemas['PartnerRef'];
export type PartnerPayment = Schemas['PartnerPayment'];
export type PartnerPaymentInput = Schemas['PartnerPaymentInput'];
export type PartnerPaymentMethod = Schemas['PartnerPaymentMethod'];
export type PartnerStatement = Schemas['PartnerStatement'];
export type PartnerStatementBooking = Schemas['PartnerStatementBooking'];
export type CustomerCategory = Schemas['CustomerCategory'];
export type CustomerCategoryInput = Schemas['CustomerCategoryInput'];
export type RegisterCategory = Schemas['RegisterCategory'];
export type RequirementVerification = Schemas['RequirementVerification'];
export type RequirementVerificationInput = Schemas['RequirementVerificationInput'];
export type AgreementVersion = Schemas['AgreementVersion'];
export type AgreementQuotaUsage = Schemas['AgreementQuotaUsage'];

export const PARTNER_KINDS: readonly PartnerKind[] = [
  'struttura_ricettiva',
  'ente',
  'azienda',
  'associazione',
  'altro',
];
export const ACCOMMODATION_TYPES: readonly AccommodationType[] = [
  'hotel',
  'bnb',
  'affittacamere',
  'campeggio',
  'casa_vacanze',
  'altro',
];
export const REGISTER_CATEGORIES: readonly RegisterCategory[] = [
  'residents',
  'accommodation',
  'protected',
  'companies',
  'associations',
  'other',
];
export const PAYMENT_METHODS: readonly PartnerPaymentMethod[] = [
  'bank_transfer',
  'cash',
  'card',
  'other',
];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export const partners = createResource<
  Partner,
  PartnerInput,
  { q?: string; kind?: string; active?: string }
>({
  key: 'partners',
  collection: `${BASE}/partners`,
  item: `${BASE}/partners/{partnerId}`,
  idParam: 'partnerId',
});

export const customerCategories = createResource<CustomerCategory, CustomerCategoryInput>({
  key: 'customer-categories',
  collection: `${BASE}/customer-categories`,
  item: `${BASE}/customer-categories/{customerCategoryId}`,
  idParam: 'customerCategoryId',
});

/** One partner (detail page). */
export function usePartner(establishmentId: string, partnerId: string | undefined) {
  const client = useApiClient();
  return useQuery<Partner, ApiProblem>({
    queryKey: [...configKey(establishmentId), 'partners', 'item', partnerId ?? ''],
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/partners/{partnerId}`, {
          params: { path: { establishmentId, partnerId: partnerId as string } },
        }),
      ),
    enabled: !!partnerId,
    retry: shouldRetry,
  });
}

/** Inclusive period (YYYY-MM-DD) of a statement or report. */
export interface Period {
  from: string;
  to: string;
}

export const accountKeys = {
  all: (est: string) => [...establishmentKeys.area(est, 'reports'), 'partner-account'] as const,
  statement: (est: string, partnerId: string, period: Period | null) =>
    [...accountKeys.all(est), 'statement', partnerId, period] as const,
  payments: (est: string, partnerId: string) =>
    [...accountKeys.all(est), 'payments', partnerId] as const,
};

const enc = encodeURIComponent;

/** Download / print URL of the statement (cookie auth): xlsx file or printable html (PDF). */
export function statementUrl(
  establishmentId: string,
  partnerId: string,
  period: Period,
  format: 'xlsx' | 'html',
): string {
  const q = new URLSearchParams({ from: period.from, to: period.to, format });
  return `/api/admin/v1/establishments/${enc(establishmentId)}/partners/${enc(partnerId)}/statement?${q.toString()}`;
}

/** Partner statement (spec 30.2.4): bookings charged, payments and balances of a period. */
export function usePartnerStatement(
  establishmentId: string,
  partnerId: string | undefined,
  period: Period | null,
  enabled = true,
) {
  const client = useApiClient();
  return useQuery<PartnerStatement, ApiProblem>({
    queryKey: accountKeys.statement(establishmentId, partnerId ?? '', period),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/partners/{partnerId}/statement`, {
          params: {
            path: { establishmentId, partnerId: partnerId as string },
            query: { from: (period as Period).from, to: (period as Period).to },
          },
        }),
      ) as Promise<PartnerStatement>,
    enabled: enabled && !!partnerId && !!period,
    placeholderData: keepPreviousData,
    retry: shouldRetry,
  });
}

function useInvalidateAccount(establishmentId: string) {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: accountKeys.all(establishmentId) }),
      // The partner list shows the balance.
      qc.invalidateQueries({ queryKey: [...configKey(establishmentId), 'partners'] }),
    ]);
}

/** Records a payment received from the partner (cash.operate); lowers the balance. */
export function useRecordPartnerPayment(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateAccount(establishmentId);
  return useMutation<
    PartnerPayment,
    ApiProblem,
    { partnerId: string; input: PartnerPaymentInput; idempotencyKey?: string }
  >({
    mutationFn: ({ partnerId, input, idempotencyKey }) =>
      call(() =>
        client.POST(`${BASE}/partners/{partnerId}/payments`, {
          params: {
            path: { establishmentId, partnerId },
            header: { 'Idempotency-Key': idempotencyKey ?? newIdempotencyKey() },
          },
          body: input,
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

/** Voids a recorded payment (the row stays with reason and operator). */
export function useVoidPartnerPayment(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateAccount(establishmentId);
  return useMutation<
    PartnerPayment,
    ApiProblem,
    { partnerId: string; paymentId: string; reason: string }
  >({
    mutationFn: ({ partnerId, paymentId, reason }) =>
      call(() =>
        client.POST(`${BASE}/partners/{partnerId}/payments/{partnerPaymentId}/void`, {
          params: { path: { establishmentId, partnerId, partnerPaymentId: paymentId } },
          body: { reason },
        }),
      ),
    onSuccess: () => void invalidate(),
  });
}

const verificationKey = (est: string, customerId: string) =>
  [...establishmentKeys.area(est, 'customers'), 'requirement-verifications', customerId] as const;

/** Requirement verifications of a customer (date and operator, never a document). */
export function useCustomerVerifications(establishmentId: string, customerId: string | undefined) {
  const client = useApiClient();
  return useQuery<RequirementVerification[], ApiProblem>({
    queryKey: verificationKey(establishmentId, customerId ?? ''),
    queryFn: async () =>
      (
        await call(() =>
          client.GET(`${BASE}/customers/{customerId}/requirement-verifications`, {
            params: { path: { establishmentId, customerId: customerId as string } },
          }),
        )
      ).items,
    enabled: !!customerId,
    retry: shouldRetry,
  });
}

export function useVerifyCustomerRequirement(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    RequirementVerification,
    ApiProblem,
    { customerId: string; input: RequirementVerificationInput }
  >({
    mutationFn: ({ customerId, input }) =>
      call(() =>
        client.POST(`${BASE}/customers/{customerId}/requirement-verifications`, {
          params: {
            path: { establishmentId, customerId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body: input,
        }),
      ),
    onSuccess: (_, { customerId }) =>
      void qc.invalidateQueries({ queryKey: verificationKey(establishmentId, customerId) }),
  });
}

/** "Verificato" on a booking: date and operator recorded with the booking's agreement. */
export function useVerifyBookingRequirement(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    RequirementVerification,
    ApiProblem,
    { bookingId: string; version: number; input?: RequirementVerificationInput }
  >({
    mutationFn: ({ bookingId, version, input }) =>
      call(() =>
        client.POST(`${BASE}/bookings/{bookingId}/requirement-verification`, {
          params: {
            path: { establishmentId, bookingId },
            header: { 'If-Match': ifMatch(version) },
          },
          body: input ?? {},
        }),
      ),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, 'bookings') }),
  });
}

/** Versions of an agreement, latest first (spec 30.2.2: history of the conditions). */
export function useAgreementVersions(establishmentId: string, agreementId: string | undefined) {
  const client = useApiClient();
  return useQuery<AgreementVersion[], ApiProblem>({
    queryKey: [...configKey(establishmentId), 'agreements', 'versions', agreementId ?? ''],
    queryFn: async () =>
      (
        await call(() =>
          client.GET(`${BASE}/agreements/{agreementId}/versions`, {
            params: { path: { establishmentId, agreementId: agreementId as string } },
          }),
        )
      ).items,
    enabled: !!agreementId,
    retry: shouldRetry,
  });
}
