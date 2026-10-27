import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { useQuery } from '@tanstack/react-query';
import { HandshakeIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useBookingEnv } from '../../flow/context';
import { PUBLIC_NS } from '../../i18n';

type PublicAgreement = Schemas['PublicAgreement'];

/** The agreement of the link (`GET /api/public/v1/{slug}/agreements/{code}`); 404 = not offered. */
function usePublicAgreement(slug: string, code: string) {
  const client = useApiClient();
  return useQuery<PublicAgreement, ApiProblem>({
    queryKey: ['public', slug, 'agreement', code],
    queryFn: () =>
      call(() =>
        client.GET('/api/public/v1/{slug}/agreements/{agreementCode}', {
          params: { path: { slug, agreementCode: code } },
        }),
      ),
    enabled: !!code,
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

/**
 * Banner of an agreement link `/b/{slug}?c={code}` (spec 30.2.2): name of the
 * agreement and of the partner, the condition in short, the validity and the
 * requirement to show at the desk. The binding outcome is the quote's
 * `agreement.status`; an unknown or expired code says so without revealing
 * whether it exists. Lazy chunk: only pages with a code load it.
 */
export default function AgreementBanner({
  code,
  status,
}: {
  code: string;
  status?: Schemas['AgreementStatus'];
}) {
  const { t } = useTranslation(PUBLIC_NS);
  const { slug } = useBookingEnv();
  const agreement = usePublicAgreement(slug, code);
  const outcome = status && status.code.toUpperCase() === code ? status.status : undefined;
  if (agreement.isPending) return null;
  if (!agreement.data || outcome === 'invalid') {
    return (
      <Alert variant="warning" role="status">
        <HandshakeIcon aria-hidden="true" />
        <AlertDescription>{t('agreement.unknown', { code })}</AlertDescription>
      </Alert>
    );
  }
  const a = agreement.data;
  const validity =
    a.valid_from && a.valid_to
      ? t('agreement.validity', {
          from: formatIsoDate(a.valid_from),
          to: formatIsoDate(a.valid_to),
        })
      : a.valid_to
        ? t('agreement.validUntil', { to: formatIsoDate(a.valid_to) })
        : '';
  return (
    <Alert role="status">
      <HandshakeIcon aria-hidden="true" />
      <AlertTitle className="line-clamp-none">
        {a.partner_name && a.partner_name !== a.name
          ? t('agreement.titlePartner', { name: a.name, partner: a.partner_name })
          : t('agreement.title', { name: a.name })}
      </AlertTitle>
      <AlertDescription>
        {a.summary && <p>{a.summary}</p>}
        {validity && <p>{validity}</p>}
        {a.requirement && <p>{t('agreement.requirement', { requirement: a.requirement })}</p>}
        {outcome === 'not_applicable' && <p>{t('agreement.notApplicable', { code })}</p>}
        {outcome === 'quota_exhausted' && <p>{t('agreement.quotaExhausted')}</p>}
      </AlertDescription>
    </Alert>
  );
}
