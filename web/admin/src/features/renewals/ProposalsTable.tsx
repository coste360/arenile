import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import {
  ConfirmDialog,
  DataTable,
  EmptyState,
  OptionSelect,
  QueryState,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  BellRingIcon,
  CalendarClockIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleAlertIcon,
  ExternalLinkIcon,
  MoreHorizontalIcon,
  SendIcon,
  UnlockIcon,
  XIcon,
} from 'lucide-react';
import {
  createContext,
  useContext,
  useMemo,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import type { ProblemText } from '../bookings/problem';
import { useEstablishmentSpots } from '../bookings/spots';
import { useDebounced } from '../customers/lib';
import {
  useBulkRenewal,
  useRenewalProposals,
  useSendProposal,
  type ProposalFilters,
  type RenewalBulkRequest,
  type RenewalBulkResult,
  type RenewalCampaign,
  type RenewalProposal,
  type RenewalProposalStatus,
} from './api';
import { Countdown } from './Countdown';
import { PROPOSAL_STATUSES, PROPOSAL_TONE } from './lib';
import { AcceptProposalDialog, DeadlineDialog, DeclineProposalDialog } from './ProposalDialogs';
import { describeM5bProblem } from './problems';

/** Row selection shared with the checkbox cells (columns stay stable, cells never remount). */
interface Selection {
  selected: ReadonlySet<string>;
  setSelected: Dispatch<SetStateAction<ReadonlySet<string>>>;
  selectable: readonly RenewalProposal[];
  actions: RowActionHandlers;
  canWrite: boolean;
  now: number;
}
const SelectionContext = createContext<Selection | null>(null);
function useSelection(): Selection {
  const ctx = useContext(SelectionContext);
  if (!ctx) throw new Error('SelectionContext missing');
  return ctx;
}

interface RowActionHandlers {
  send: (p: RenewalProposal) => void;
  accept: (p: RenewalProposal) => void;
  decline: (p: RenewalProposal) => void;
  establishmentId: string;
}

function SelectAllCell() {
  const { t } = useTranslation('renewals');
  const { selected, setSelected, selectable } = useSelection();
  const all = selectable.length > 0 && selectable.every((p) => selected.has(p.id));
  const some = selectable.some((p) => selected.has(p.id));
  return (
    <Checkbox
      aria-label={t('bulk.selectAll')}
      checked={all ? true : some ? 'indeterminate' : false}
      disabled={selectable.length === 0}
      onCheckedChange={(v) => setSelected(v ? new Set(selectable.map((p) => p.id)) : new Set())}
    />
  );
}

function SelectCell({ proposal }: { proposal: RenewalProposal }) {
  const { t } = useTranslation('renewals');
  const { selected, setSelected } = useSelection();
  return (
    <Checkbox
      aria-label={t('bulk.selectOne', {
        spot: proposal.spot_label,
        customer: proposal.customer_display_name,
      })}
      checked={selected.has(proposal.id)}
      onClick={(e) => e.stopPropagation()}
      onCheckedChange={(v) =>
        setSelected((prev) => {
          const next = new Set(prev);
          if (v) next.add(proposal.id);
          else next.delete(proposal.id);
          return next;
        })
      }
    />
  );
}

function DeadlineCell({ proposal }: { proposal: RenewalProposal }) {
  const { now } = useSelection();
  const fmt = useInstantFormat();
  if (proposal.status === 'proposed')
    return <Countdown deadline={proposal.deadline_at} now={now} />;
  return (
    <span className="text-muted-foreground tabular-nums">
      {proposal.responded_at ? fmt.date(proposal.responded_at) : '—'}
    </span>
  );
}

function ActionsCell({ proposal: p }: { proposal: RenewalProposal }) {
  const { t } = useTranslation('renewals');
  const { actions, canWrite } = useSelection();
  const open = p.status === 'proposed';
  return (
    <div className="flex justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('row.more', { spot: p.spot_label, customer: p.customer_display_name })}
          >
            <MoreHorizontalIcon aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {canWrite && open && (
            <>
              <DropdownMenuItem
                disabled={p.customer_has_email === false}
                onSelect={() => actions.send(p)}
              >
                {p.send_count > 0 ? (
                  <BellRingIcon aria-hidden="true" />
                ) : (
                  <SendIcon aria-hidden="true" />
                )}
                {p.send_count > 0 ? t('row.remind') : t('row.send')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => actions.accept(p)}>
                <CheckIcon aria-hidden="true" />
                {t('row.accept')}
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onSelect={() => actions.decline(p)}>
                <XIcon aria-hidden="true" />
                {t('row.decline')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          <DropdownMenuItem asChild>
            <Link
              to="/e/$establishmentId/prenotazioni/$bookingId"
              params={{ establishmentId: actions.establishmentId, bookingId: p.booking_id }}
            >
              <ExternalLinkIcon aria-hidden="true" />
              {t('row.openBooking')}
            </Link>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

type SentFilter = '' | 'yes' | 'no';

type BulkScope =
  | { kind: 'selection'; action: 'send' | 'accept' | 'decline' | 'release' }
  | { kind: 'unsent' | 'remind' | 'releaseExpired' };

export interface ProposalsTableProps {
  establishmentId: string;
  campaign: RenewalCampaign;
  canWrite: boolean;
  now: number;
  /** Last allowed deadline date (the day before the target season opens). */
  maxDeadline?: string;
}

/**
 * Proposals of a campaign (spec 10.4): server-side filters (status, sent,
 * zone, search), row actions (send / remind, accept, decline, open the
 * booking), selection and bulk actions with per-proposal failures.
 */
export function ProposalsTable({
  establishmentId,
  campaign,
  canWrite,
  now,
  maxDeadline,
}: ProposalsTableProps) {
  const { t, i18n } = useTranslation('renewals');
  const [status, setStatus] = useState<'' | RenewalProposalStatus>('');
  const [sent, setSent] = useState<SentFilter>('');
  const [zoneId, setZoneId] = useState('');
  const [search, setSearch] = useState('');
  const q = useDebounced(search.trim());
  const filters: ProposalFilters = {
    status: status || undefined,
    sent: sent === '' ? undefined : sent === 'yes',
    zone_id: zoneId || undefined,
    q: q || undefined,
  };
  const list = useRenewalProposals(establishmentId, campaign.id, filters);
  const proposals = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data]);
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const selectable = useMemo(
    () => proposals.filter((p) => p.status === 'proposed' || p.status === 'accepted'),
    [proposals],
  );
  const selectedIds = selectable.filter((p) => selected.has(p.id)).map((p) => p.id);

  const send = useSendProposal(establishmentId);
  const bulk = useBulkRenewal(establishmentId, campaign.id);
  const [accepting, setAccepting] = useState<RenewalProposal | null>(null);
  const [declining, setDeclining] = useState<RenewalProposal | null>(null);
  const [confirming, setConfirming] = useState<BulkScope | null>(null);
  const [extending, setExtending] = useState(false);
  const [extendFailure, setExtendFailure] = useState<ProblemText | null>(null);
  const [bulkResult, setBulkResult] = useState<RenewalBulkResult | null>(null);
  const campaignOpen = campaign.status === 'open';
  const writable = canWrite && campaignOpen;

  const sendMutate = send.mutate;
  const actions = useMemo<RowActionHandlers>(
    () => ({
      establishmentId,
      send: (p) =>
        sendMutate(p, {
          onSuccess: () => toast.success(t('row.sent', { customer: p.customer_display_name })),
          onError: (err) => toast.error(describeM5bProblem(err, t).title),
        }),
      accept: setAccepting,
      decline: setDeclining,
    }),
    [establishmentId, t, sendMutate],
  );

  const reportBulk = (r: RenewalBulkResult) => {
    setBulkResult(r.failed.length > 0 ? r : null);
    toast.success(
      t('bulk.result', { succeeded: r.succeeded, skipped: r.skipped, failed: r.failed.length }),
    );
    setSelected(new Set());
  };

  const bulkRequest = (scope: BulkScope): RenewalBulkRequest => {
    switch (scope.kind) {
      case 'selection':
        return {
          action: scope.action,
          proposal_ids: selectedIds,
          only_unsent: scope.action === 'send' ? false : undefined,
          force: scope.action === 'release' ? true : undefined,
        };
      case 'unsent':
        return { action: 'send', status: 'proposed', only_unsent: true };
      case 'remind':
        return { action: 'send', status: 'proposed', only_unsent: false };
      case 'releaseExpired':
        return { action: 'release', status: 'proposed', force: false };
    }
  };

  const confirmText = (scope: BulkScope): { title: string; description: string } => {
    const key = scope.kind === 'selection' ? `selection.${scope.action}` : scope.kind;
    return {
      title: t(`bulk.confirm.${key}.title`),
      description: t(`bulk.confirm.${key}.description`, { count: selectedIds.length }),
    };
  };

  const zoneName = (id?: string) => (id ? inventory.zoneName(id) : '');

  const columns = useMemo<ColumnDef<RenewalProposal>[]>(
    () => [
      {
        id: 'select',
        enableHiding: false,
        header: () => <SelectAllCell />,
        cell: ({ row }) =>
          row.original.status === 'proposed' || row.original.status === 'accepted' ? (
            <SelectCell proposal={row.original} />
          ) : null,
      },
      {
        id: 'spot',
        header: t('columns.spot'),
        meta: { label: t('columns.spot') },
        cell: ({ row }) => (
          <span className="font-medium whitespace-nowrap">{row.original.spot_label}</span>
        ),
      },
      {
        id: 'customer',
        header: t('columns.customer'),
        meta: { label: t('columns.customer') },
        cell: ({ row }) => (
          <span className="flex flex-wrap items-center gap-2">
            {row.original.customer_display_name}
            {row.original.customer_has_email === false && (
              <Badge variant="outline">{t('row.noEmail')}</Badge>
            )}
          </span>
        ),
      },
      {
        id: 'amount',
        header: t('columns.amount'),
        meta: { label: t('columns.amount'), align: 'right' },
        cell: ({ row: { original: p } }) => (
          <span className="flex flex-col items-end">
            <span>{formatCents(p.total_amount)}</span>
            <span className="text-xs text-muted-foreground">
              {t('row.previous', { amount: formatCents(p.previous_amount) })}
            </span>
          </span>
        ),
      },
      {
        id: 'status',
        header: t('columns.status'),
        meta: { label: t('columns.status') },
        cell: ({ row: { original: p } }) => (
          <StatusBadge tone={PROPOSAL_TONE[p.status]}>{t(`status.${p.status}`)}</StatusBadge>
        ),
      },
      {
        id: 'sent',
        header: t('columns.sent'),
        meta: { label: t('columns.sent') },
        cell: ({ row: { original: p } }) =>
          p.send_count > 0 ? (
            <span className="whitespace-nowrap">{t('row.sentCount', { count: p.send_count })}</span>
          ) : (
            <span className="text-muted-foreground">{t('row.notSent')}</span>
          ),
      },
      {
        id: 'deadline',
        header: t('columns.deadline'),
        meta: { label: t('columns.deadline') },
        cell: ({ row }) => <DeadlineCell proposal={row.original} />,
      },
      {
        id: 'actions',
        enableHiding: false,
        header: () => <span className="sr-only">{t('columns.actions')}</span>,
        cell: ({ row }) => <ActionsCell proposal={row.original} />,
      },
    ],
    [t],
  );

  const statusOptions = [
    { value: '', label: t('filters.allStatuses') },
    ...PROPOSAL_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
  ];
  const zoneOptions = [
    { value: '', label: t('filters.allZones') },
    ...inventory.zones.map((z) => ({ value: z.id, label: zoneName(z.id) || z.code })),
  ];

  const bulkMenu: ReactNode = writable && (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          {selectedIds.length > 0
            ? t('bulk.selected', { count: selectedIds.length })
            : t('bulk.menu')}
          <ChevronDownIcon aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {selectedIds.length > 0 ? (
          <>
            <DropdownMenuLabel>{t('bulk.onSelection')}</DropdownMenuLabel>
            <DropdownMenuItem onSelect={() => setConfirming({ kind: 'selection', action: 'send' })}>
              <SendIcon aria-hidden="true" />
              {t('bulk.send')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => setConfirming({ kind: 'selection', action: 'accept' })}
            >
              <CheckIcon aria-hidden="true" />
              {t('bulk.accept')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => {
                setExtendFailure(null);
                setExtending(true);
              }}
            >
              <CalendarClockIcon aria-hidden="true" />
              {t('bulk.extend')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setConfirming({ kind: 'selection', action: 'decline' })}
            >
              <XIcon aria-hidden="true" />
              {t('bulk.decline')}
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setConfirming({ kind: 'selection', action: 'release' })}
            >
              <UnlockIcon aria-hidden="true" />
              {t('bulk.release')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setSelected(new Set())}>
              {t('bulk.clear')}
            </DropdownMenuItem>
          </>
        ) : (
          <>
            <DropdownMenuItem onSelect={() => setConfirming({ kind: 'unsent' })}>
              <SendIcon aria-hidden="true" />
              {t('bulk.sendUnsent')}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setConfirming({ kind: 'remind' })}>
              <BellRingIcon aria-hidden="true" />
              {t('bulk.remind')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setConfirming({ kind: 'releaseExpired' })}
            >
              <UnlockIcon aria-hidden="true" />
              {t('bulk.releaseExpired')}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );

  const failedLabel = (id: string) => {
    const p = proposals.find((x) => x.id === id);
    return p ? `${p.spot_label} · ${p.customer_display_name}` : id;
  };

  return (
    <SelectionContext.Provider
      value={{ selected, setSelected, selectable, actions, canWrite: writable, now }}
    >
      {bulkResult && (
        <Alert variant="destructive" role="alert">
          <CircleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('bulk.failedTitle', { count: bulkResult.failed.length })}</AlertTitle>
          <AlertDescription>
            <ul className="list-inside list-disc">
              {bulkResult.failed.map((f) => (
                <li key={f.proposal_id}>
                  {failedLabel(f.proposal_id)}:{' '}
                  {t(`problem.${f.type}`, {
                    defaultValue: f.detail ?? t('common.errors.generic', { ns: 'translation' }),
                  })}
                </li>
              ))}
            </ul>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => setBulkResult(null)}
            >
              {t('bulk.dismiss')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <DataTable
        caption={t('proposals.title')}
        columns={columns}
        data={proposals}
        loading={list.isPending}
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        getRowId={(p) => p.id}
        pageSize={0}
        searchValue={search}
        onSearchChange={setSearch}
        searchPlaceholder={t('filters.search')}
        filters={
          <>
            <OptionSelect
              aria-label={t('filters.status')}
              value={status}
              onChange={(v) => setStatus(v as '' | RenewalProposalStatus)}
              options={statusOptions}
              className="w-full sm:w-44"
            />
            <OptionSelect
              aria-label={t('filters.sent')}
              value={sent}
              onChange={(v) => setSent(v as SentFilter)}
              options={[
                { value: '', label: t('filters.sentAll') },
                { value: 'yes', label: t('filters.sentYes') },
                { value: 'no', label: t('filters.sentNo') },
              ]}
              className="w-full sm:w-40"
            />
            {inventory.zones.length > 1 && (
              <OptionSelect
                aria-label={t('filters.zone')}
                value={zoneId}
                onChange={setZoneId}
                options={zoneOptions}
                className="w-full sm:w-44"
              />
            )}
          </>
        }
        toolbar={bulkMenu}
        loadMore={{
          hasMore: !!list.hasNextPage,
          loading: list.isFetchingNextPage,
          onLoadMore: () => void list.fetchNextPage(),
          label: t('proposals.loadMore'),
        }}
        empty={
          <EmptyState
            icon={SendIcon}
            bordered={false}
            title={t('proposals.empty')}
            description={t('proposals.emptyHint')}
          />
        }
      />
      <AcceptProposalDialog
        establishmentId={establishmentId}
        proposal={accepting}
        onClose={() => setAccepting(null)}
      />
      <DeclineProposalDialog
        establishmentId={establishmentId}
        proposal={declining}
        onClose={() => setDeclining(null)}
      />
      {confirming && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirming(null)}
          {...confirmText(confirming)}
          destructive={
            (confirming.kind === 'selection' &&
              (confirming.action === 'decline' || confirming.action === 'release')) ||
            confirming.kind === 'releaseExpired'
          }
          errorText={(err) => describeM5bProblem(err, t).title}
          onConfirm={async () => {
            const r = await bulk.mutateAsync(bulkRequest(confirming));
            reportBulk(r);
          }}
        />
      )}
      {extending && (
        <DeadlineDialog
          open
          onOpenChange={setExtending}
          title={t('bulk.extendTitle')}
          description={t('bulk.extendDescription', { count: selectedIds.length })}
          current={campaign.deadline_at}
          maxDate={maxDeadline}
          pending={bulk.isPending}
          failure={extendFailure}
          onSubmit={(deadlineAt) =>
            bulk.mutate(
              { action: 'extend_deadline', proposal_ids: selectedIds, deadline_at: deadlineAt },
              {
                onSuccess: (r) => {
                  reportBulk(r);
                  setExtending(false);
                },
                onError: (err) => setExtendFailure(describeM5bProblem(err, t)),
              },
            )
          }
        />
      )}
    </SelectionContext.Provider>
  );
}
