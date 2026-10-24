import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';

/**
 * Data hooks of the online configuration pages (spec 12, 13, 19): public
 * page settings and terms versions, payment provider settings, email
 * settings and message log. Settings use If-Match with the version read
 * (412 version_conflict on a stale edit).
 */

export type OnlineSettings = Schemas['OnlineSettings'];
export type OnlineSettingsInput = Schemas['OnlineSettingsInput'];
export type TermsVersion = Schemas['TermsVersion'];
export type TermsVersionInput = Schemas['TermsVersionInput'];
export type PaymentSettings = Schemas['PaymentSettings'];
export type PaymentSettingsInput = Schemas['PaymentSettingsInput'];
export type PaymentSettingsTestResult = Schemas['PaymentSettingsTestResult'];
export type EmailSettings = Schemas['EmailSettings'];
export type EmailSettingsInput = Schemas['EmailSettingsInput'];
export type MessageLogEntry = Schemas['MessageLogEntry'];
export type MessageStatus = Schemas['MessageStatus'];
export type EmailTemplate = Schemas['EmailTemplate'];
export type EmailTemplateTexts = Schemas['EmailTemplateTexts'];
export type EmailTemplateKind = Schemas['EmailTemplateKind'];
export type EmailLocale = Schemas['EmailLocale'];
export type EmailTemplateList = Schemas['EmailTemplateList'];
export type EmailTemplatePreview = Schemas['EmailTemplatePreview'];

export const onlineKeys = {
  settings: (id: string) => ['online', id, 'settings'] as const,
  terms: (id: string) => ['online', id, 'terms'] as const,
  payments: (id: string) => ['online', id, 'payments'] as const,
  email: (id: string) => ['online', id, 'email'] as const,
  messages: (id: string, status?: string) => ['online', id, 'messages', status ?? ''] as const,
  templates: (id: string) => ['online', id, 'email-templates'] as const,
};

const path = (establishmentId: string) => ({ path: { establishmentId } });

export function useOnlineSettings(establishmentId: string) {
  const client = useApiClient();
  return useQuery<OnlineSettings, ApiProblem>({
    queryKey: onlineKeys.settings(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/online-settings', {
          params: path(establishmentId),
        }),
      ),
    retry: shouldRetry,
  });
}

export function useUpdateOnlineSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<OnlineSettings, ApiProblem, { version: number; body: OnlineSettingsInput }>({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT('/api/admin/v1/establishments/{establishmentId}/online-settings', {
          params: { ...path(establishmentId), header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => {
      qc.setQueryData(onlineKeys.settings(establishmentId), s);
      // Theme and booking switch also live on the establishment.
      void qc.invalidateQueries({ queryKey: ['establishment', establishmentId] });
    },
  });
}

export function useTermsVersions(establishmentId: string) {
  const client = useApiClient();
  return useQuery<TermsVersion[], ApiProblem>({
    queryKey: onlineKeys.terms(establishmentId),
    queryFn: async () =>
      (
        await call(() =>
          client.GET('/api/admin/v1/establishments/{establishmentId}/terms-versions', {
            params: path(establishmentId),
          }),
        )
      ).items,
    retry: shouldRetry,
  });
}

export function usePublishTerms(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<TermsVersion, ApiProblem, TermsVersionInput>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/terms-versions', {
          params: { ...path(establishmentId), header: { 'Idempotency-Key': newIdempotencyKey() } },
          body,
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: onlineKeys.terms(establishmentId) });
      void qc.invalidateQueries({ queryKey: onlineKeys.settings(establishmentId) });
    },
  });
}

export function usePaymentSettings(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<PaymentSettings, ApiProblem>({
    queryKey: onlineKeys.payments(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/payment-settings', {
          params: path(establishmentId),
        }),
      ),
    enabled,
    retry: shouldRetry,
  });
}

export function useUpdatePaymentSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<PaymentSettings, ApiProblem, { version: number; body: PaymentSettingsInput }>({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT('/api/admin/v1/establishments/{establishmentId}/payment-settings', {
          params: { ...path(establishmentId), header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => qc.setQueryData(onlineKeys.payments(establishmentId), s),
  });
}

export function useTestPaymentSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<PaymentSettingsTestResult, ApiProblem, void>({
    mutationFn: () =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/payment-settings/test', {
          params: path(establishmentId),
        }),
      ),
    // A successful check stores verified_at.
    onSuccess: () => void qc.invalidateQueries({ queryKey: onlineKeys.payments(establishmentId) }),
  });
}

export function useEmailSettings(establishmentId: string) {
  const client = useApiClient();
  return useQuery<EmailSettings, ApiProblem>({
    queryKey: onlineKeys.email(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/email-settings', {
          params: path(establishmentId),
        }),
      ),
    retry: shouldRetry,
  });
}

export function useUpdateEmailSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<EmailSettings, ApiProblem, { version: number; body: EmailSettingsInput }>({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT('/api/admin/v1/establishments/{establishmentId}/email-settings', {
          params: { ...path(establishmentId), header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => qc.setQueryData(onlineKeys.email(establishmentId), s),
  });
}

export function useSendTestEmail(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<Schemas['MessageQueued'], ApiProblem, { to: string }>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/email-settings/test', {
          params: path(establishmentId),
          body,
        }),
      ),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: ['online', establishmentId, 'messages'] }),
  });
}

export function useMessages(establishmentId: string, status?: MessageStatus) {
  const client = useApiClient();
  return useQuery<Schemas['MessageLogList'], ApiProblem>({
    queryKey: onlineKeys.messages(establishmentId, status),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/messages', {
          params: { ...path(establishmentId), query: { status, limit: 50 } },
        }),
      ),
    retry: shouldRetry,
    refetchInterval: 15_000,
  });
}

// ---- editable customer email texts (ADR 0022) ---------------------------------

const templatePath = (establishmentId: string, kind: EmailTemplateKind, locale: EmailLocale) => ({
  path: { establishmentId, kind, locale },
});

export function useEmailTemplates(establishmentId: string) {
  const client = useApiClient();
  return useQuery<EmailTemplateList, ApiProblem>({
    queryKey: onlineKeys.templates(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/email-templates', {
          params: path(establishmentId),
        }),
      ),
    retry: shouldRetry,
  });
}

interface TemplateTarget {
  kind: EmailTemplateKind;
  locale: EmailLocale;
}

/** Replaces one template in the cached list (save and restore answers). */
function storeTemplate(
  qc: ReturnType<typeof useQueryClient>,
  establishmentId: string,
  t: EmailTemplate,
) {
  qc.setQueryData<EmailTemplateList>(onlineKeys.templates(establishmentId), (old) =>
    old
      ? {
          ...old,
          items: old.items.map((i) => (i.kind === t.kind && i.locale === t.locale ? t : i)),
        }
      : old,
  );
}

export function useUpdateEmailTemplate(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    EmailTemplate,
    ApiProblem,
    TemplateTarget & { version: number; body: EmailTemplateTexts }
  >({
    mutationFn: ({ kind, locale, version, body }) =>
      call(() =>
        client.PUT(
          '/api/admin/v1/establishments/{establishmentId}/email-templates/{kind}/{locale}',
          {
            params: {
              ...templatePath(establishmentId, kind, locale),
              header: { 'If-Match': ifMatch(version) },
            },
            body,
          },
        ),
      ),
    onSuccess: (t) => storeTemplate(qc, establishmentId, t),
  });
}

export function useResetEmailTemplate(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<EmailTemplate, ApiProblem, TemplateTarget & { version: number }>({
    mutationFn: ({ kind, locale, version }) =>
      call(() =>
        client.DELETE(
          '/api/admin/v1/establishments/{establishmentId}/email-templates/{kind}/{locale}',
          {
            params: {
              ...templatePath(establishmentId, kind, locale),
              header: { 'If-Match': ifMatch(version) },
            },
          },
        ),
      ),
    onSuccess: (t) => storeTemplate(qc, establishmentId, t),
  });
}

export function usePreviewEmailTemplate(establishmentId: string) {
  const client = useApiClient();
  return useMutation<
    EmailTemplatePreview,
    ApiProblem,
    TemplateTarget & { body: EmailTemplateTexts }
  >({
    mutationFn: ({ kind, locale, body }) =>
      call(() =>
        client.POST(
          '/api/admin/v1/establishments/{establishmentId}/email-templates/{kind}/{locale}/preview',
          { params: templatePath(establishmentId, kind, locale), body },
        ),
      ),
  });
}

export function useSendEmailTemplateTest(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<
    Schemas['MessageQueued'],
    ApiProblem,
    TemplateTarget & { body: EmailTemplateTexts }
  >({
    mutationFn: ({ kind, locale, body }) =>
      call(() =>
        client.POST(
          '/api/admin/v1/establishments/{establishmentId}/email-templates/{kind}/{locale}/test',
          { params: templatePath(establishmentId, kind, locale), body },
        ),
      ),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: ['online', establishmentId, 'messages'] }),
  });
}
