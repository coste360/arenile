import { partners as partnersResource } from '../../partners/api';
import { agreements as agreementsResource, type Agreement } from '../../pricing/api';
import { deskAgreements } from './model';

export interface AgreementOptions {
  agreements: Agreement[];
  partners: { id: string; name: string; active: boolean }[];
  /** The chosen agreement (for its requirement and partner). */
  selected?: Agreement;
}

/** Agreements and partners for the selectors, keeping the current ones even if inactive. */
export function useAgreementOptions(
  establishmentId: string,
  agreementId: string,
  partnerId: string,
): AgreementOptions {
  const agreementList = agreementsResource.useList(establishmentId);
  const partnerList = partnersResource.useList(establishmentId);
  const all = agreementList.data ?? [];
  return {
    agreements: deskAgreements(all, agreementId),
    partners: (partnerList.data ?? []).filter((p) => p.active || p.id === partnerId),
    selected: all.find((a) => a.id === agreementId),
  };
}
