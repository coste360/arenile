/**
 * Server state of the renewals feature (M5B: renewal campaigns and proposals (spec 10.4)): types from the
 * generated schema. The renewals frontend builder adds the query key factory
 * (establishmentKeys.area(id, 'renewals')) and the TanStack Query hooks here.
 */
import type { Schemas } from '@arenile/shared/api';

export type RenewalCampaign = Schemas['RenewalCampaign'];
export type RenewalCampaignList = Schemas['RenewalCampaignList'];
export type RenewalCampaignCreate = Schemas['RenewalCampaignCreate'];
export type RenewalCampaignUpdate = Schemas['RenewalCampaignUpdate'];
export type RenewalCampaignResult = Schemas['RenewalCampaignResult'];
export type RenewalCampaignStatus = Schemas['RenewalCampaignStatus'];
export type RenewalProposal = Schemas['RenewalProposal'];
export type RenewalProposalList = Schemas['RenewalProposalList'];
export type RenewalProposalStatus = Schemas['RenewalProposalStatus'];
export type RenewalBulkRequest = Schemas['RenewalBulkRequest'];
export type RenewalBulkResult = Schemas['RenewalBulkResult'];
export type RenewalBulkAction = Schemas['RenewalBulkAction'];
export type RenewalStaffAcceptRequest = Schemas['RenewalStaffAcceptRequest'];
export type RenewalDeclineRequest = Schemas['RenewalDeclineRequest'];
export type RenewalSkipped = Schemas['RenewalSkipped'];
