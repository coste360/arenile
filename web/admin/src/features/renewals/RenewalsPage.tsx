import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import {
  ConfirmDialog,
  EmptyState,
  OptionSelect,
  Page,
  PageToolbar,
  QueryState,
  SectionCard,
  StatCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { Link, useSearch } from '@tanstack/react-router';
import {
  CalendarClockIcon,
  CheckCircle2Icon,
  HourglassIcon,
  LockOpenIcon,
  PlusIcon,
  RefreshCwIcon,
  SendIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import { useOperationalPermissions } from '../bookings/model';
import type { ProblemText } from '../bookings/problem';
import { seasons as seasonsResource, type Season } from '../configuration/api/resources';
import { useRenewalCampaigns, useUpdateRenewalCampaign, type RenewalCampaign } from './api';
import { CampaignSheet } from './CampaignSheet';
import { Countdown } from './Countdown';
import { CAMPAIGN_TONE, useNow } from './lib';
import { DeadlineDialog } from './ProposalDialogs';
import { ProposalsTable } from './ProposalsTable';
import { describeM5bProblem } from './problems';

/** Open campaign with the nearest deadline, else the most recent one. */
function defaultCampaign(items: RenewalCampaign[]): RenewalCampaign | undefined {
  const open = items
    .filter((c) => c.status === 'open')
    .sort((a, b) => a.deadline_at.localeCompare(b.deadline_at));
  return open[0] ?? [...items].sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
}

function dayBefore(iso: string): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() - 1);
  return d.toISOString().slice(0, 10);
}

/**
 * Rinnovi (spec 10.4, 15.1): renewal campaigns of the season passes with
 * right of pre-emption. KPIs, deadline countdown, proposals with filters,
 * row and bulk actions; "Nuova campagna" previews then creates the proposals
 * (each one a held booking protecting the spot until the deadline, ADR 0017).
 */
export function RenewalsPage() {
  const { t } = useTranslation('renewals');
  const establishmentId = useRequiredEstablishmentId();
  const perms = useOperationalPermissions();
  const fmt = useInstantFormat();
  const now = useNow();
  const search = useSearch({ strict: false }) as {
    source?: string;
    target?: string;
    campaign?: string;
  };
  const campaigns = useRenewalCampaigns(establishmentId);
  const seasonList = seasonsResource.useList(establishmentId);
  const seasons = [...(seasonList.data ?? [])].sort((a, b) => b.year - a.year);
  const items = campaigns.data?.items ?? [];
  const [choice, setChoice] = useState(search.campaign ?? '');
  const campaign = items.find((c) => c.id === choice) ?? defaultCampaign(items);
  const [creating, setCreating] = useState(!!search.target);
  const [closing, setClosing] = useState(false);
  const [extending, setExtending] = useState(false);
  const [extendFailure, setExtendFailure] = useState<ProblemText | null>(null);
  const update = useUpdateRenewalCampaign(establishmentId);
  const seasonById = new Map(seasons.map((s) => [s.id, s]));
  const year = (id: string) => seasonById.get(id)?.year ?? '…';
  const target: Season | undefined = campaign
    ? seasonById.get(campaign.target_season_id)
    : undefined;

  const createButton = perms.canWrite && (
    <Button onClick={() => setCreating(true)} disabled={seasons.length < 2}>
      <PlusIcon aria-hidden="true" />
      {t('create.title')}
    </Button>
  );

  let content: ReactNode;
  if (campaigns.isPending || campaigns.error) {
    content = (
      <QueryState
        isLoading={campaigns.isPending}
        error={campaigns.error}
        onRetry={() => void campaigns.refetch()}
      />
    );
  } else if (!campaign) {
    content = (
      <EmptyState
        icon={RefreshCwIcon}
        title={t('empty.title')}
        description={t('empty.description')}
        action={
          <div className="flex flex-wrap justify-center gap-2">
            <Button variant="outline" asChild>
              <Link to="/e/$establishmentId/config/seasons/rollover" params={{ establishmentId }}>
                {t('empty.rollover')}
              </Link>
            </Button>
            {createButton}
          </div>
        }
      />
    );
  } else {
    const c = campaign.counts;
    const total = c.proposed + c.accepted + c.declined + c.expired + c.converted;
    const open = campaign.status === 'open';
    content = (
      <>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            label={t('stats.proposals')}
            value={total}
            icon={SendIcon}
            hint={t('stats.sent', { count: c.sent })}
          />
          <StatCard
            label={t('stats.waiting')}
            value={c.proposed}
            icon={HourglassIcon}
            hint={open ? <Countdown deadline={campaign.deadline_at} now={now} /> : undefined}
          />
          <StatCard
            label={t('stats.renewed')}
            value={c.accepted + c.converted}
            icon={CheckCircle2Icon}
            tone="success"
            hint={t('stats.converted', { count: c.converted })}
          />
          <StatCard
            label={t('stats.released')}
            value={c.declined + c.expired}
            icon={LockOpenIcon}
            hint={t('stats.releasedHint', { declined: c.declined, expired: c.expired })}
          />
        </div>

        <SectionCard
          title={campaign.name}
          description={t('campaign.seasons', {
            source: year(campaign.source_season_id),
            target: year(campaign.target_season_id),
          })}
          icon={RefreshCwIcon}
          actions={
            perms.canWrite &&
            open && (
              <>
                <Button
                  variant="outline"
                  onClick={() => {
                    setExtendFailure(null);
                    setExtending(true);
                  }}
                >
                  <CalendarClockIcon aria-hidden="true" />
                  {t('campaign.changeDeadline')}
                </Button>
                <Button variant="destructive" onClick={() => setClosing(true)}>
                  {t('campaign.close')}
                </Button>
              </>
            )
          }
        >
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('campaign.status')}</dt>
              <dd>
                <StatusBadge tone={CAMPAIGN_TONE[campaign.status]}>
                  {t(`campaignStatus.${campaign.status}`)}
                </StatusBadge>
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('campaign.deadline')}</dt>
              <dd className="tabular-nums">{fmt.dateTime(campaign.deadline_at)}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('campaign.total')}</dt>
              <dd className="tabular-nums">{formatCents(campaign.total_amount)}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('campaign.converted')}</dt>
              <dd className="tabular-nums">{formatCents(campaign.converted_amount)}</dd>
            </div>
          </dl>
          {campaign.notes && (
            <p className="mt-3 text-sm whitespace-pre-line text-muted-foreground">
              {campaign.notes}
            </p>
          )}
        </SectionCard>

        <ProposalsTable
          key={campaign.id}
          establishmentId={establishmentId}
          campaign={campaign}
          canWrite={perms.canWrite}
          now={now}
          maxDeadline={target ? dayBefore(target.opens_on) : undefined}
        />

        <ConfirmDialog
          open={closing}
          onOpenChange={setClosing}
          title={t('campaign.closeTitle')}
          description={t('campaign.closeDescription', { count: c.proposed + c.accepted })}
          confirmLabel={t('campaign.close')}
          destructive
          errorText={(err) => describeM5bProblem(err, t).title}
          onConfirm={async () => {
            await update.mutateAsync({
              id: campaign.id,
              version: campaign.version,
              body: { status: 'closed' },
            });
            toast.success(t('campaign.closed'));
          }}
        />
        {extending && (
          <DeadlineDialog
            open
            onOpenChange={setExtending}
            title={t('campaign.changeDeadline')}
            description={t('campaign.changeDeadlineHelp')}
            current={campaign.deadline_at}
            maxDate={target ? dayBefore(target.opens_on) : undefined}
            pending={update.isPending}
            failure={extendFailure}
            onSubmit={(deadlineAt) =>
              update.mutate(
                { id: campaign.id, version: campaign.version, body: { deadline_at: deadlineAt } },
                {
                  onSuccess: () => {
                    toast.success(t('campaign.deadlineChanged'));
                    setExtending(false);
                  },
                  onError: (err) => setExtendFailure(describeM5bProblem(err, t)),
                },
              )
            }
          />
        )}
      </>
    );
  }

  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={campaign ? createButton : undefined}
    >
      {items.length > 1 && (
        <PageToolbar>
          <OptionSelect
            aria-label={t('campaign.pick')}
            value={campaign?.id ?? ''}
            onChange={setChoice}
            options={items.map((c) => ({
              value: c.id,
              label: `${c.name} · ${t(`campaignStatus.${c.status}`)}`,
            }))}
            className="w-full sm:w-80"
          />
        </PageToolbar>
      )}
      {content}
      <CampaignSheet
        establishmentId={establishmentId}
        open={creating}
        onOpenChange={setCreating}
        seasons={seasons}
        defaults={{ sourceSeasonId: search.source, targetSeasonId: search.target }}
        onCreated={(created) => setChoice(created.id)}
      />
    </Page>
  );
}
