import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import {
  DataTable,
  DatePicker,
  FormSheet,
  OptionSelect,
  SheetForm,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { EyeIcon, InfoIcon } from 'lucide-react';
import { useId, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment } from '../../establishment';
import { todayIso } from '../bookings/model';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import type { Season } from '../configuration/api/resources';
import { newIdempotencyKey } from '../configuration/lib/validation';
import {
  useCreateRenewalCampaign,
  type RenewalCampaign,
  type RenewalCampaignCreate,
  type RenewalCampaignResult,
  type RenewalProposalPreview,
  type RenewalSkipped,
} from './api';
import { zonedInstant } from './lib';
import { describeM5bProblem } from './problems';

export interface CampaignSheetProps {
  establishmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Seasons, newest first. */
  seasons: Season[];
  defaults?: { sourceSeasonId?: string; targetSeasonId?: string };
  onCreated: (campaign: RenewalCampaign) => void;
}

function addDays(iso: string, days: number): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** Default deadline: in 30 days, at the latest the day before the target season opens. */
function defaultDeadline(target: Season | undefined): string {
  const in30 = addDays(todayIso(), 30);
  if (!target) return in30;
  const last = addDays(target.opens_on, -1);
  return last < in30 ? last : in30;
}

/**
 * "Nuova campagna" (spec 10.4): source and target season, pre-emption
 * deadline, optional name and notes; "Anteprima" shows the proposals that
 * would be created and the passes skipped (dry run) before creating them.
 */
export function CampaignSheet(props: CampaignSheetProps) {
  const { t } = useTranslation('renewals');
  return (
    <FormSheet
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('create.title')}
      description={t('create.description')}
      size="lg"
      padded={false}
    >
      {props.open && props.seasons.length > 0 && <CampaignForm {...props} />}
    </FormSheet>
  );
}

function CampaignForm({
  establishmentId,
  onOpenChange,
  seasons,
  defaults,
  onCreated,
}: CampaignSheetProps) {
  const { t } = useTranslation('renewals');
  const { establishment } = useCurrentEstablishment();
  const tz = establishment?.timezone ?? 'Europe/Rome';
  const id = useId();
  const create = useCreateRenewalCampaign(establishmentId);
  const [createKey] = useState(newIdempotencyKey);
  const initialTarget =
    seasons.find((s) => s.id === defaults?.targetSeasonId) ?? seasons[0] ?? undefined;
  const initialSource =
    seasons.find((s) => s.id === defaults?.sourceSeasonId) ??
    seasons.find((s) => initialTarget && s.year < initialTarget.year);
  const [sourceId, setSourceId] = useState(initialSource?.id ?? '');
  const [targetId, setTargetId] = useState(initialTarget?.id ?? '');
  const [deadline, setDeadline] = useState(defaultDeadline(initialTarget));
  const [name, setName] = useState('');
  const [notes, setNotes] = useState('');
  const [preview, setPreview] = useState<{ key: string; result: RenewalCampaignResult } | null>(
    null,
  );
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const [invalid, setInvalid] = useState(false);

  const target = seasons.find((s) => s.id === targetId);
  const source = seasons.find((s) => s.id === sourceId);
  const valid = !!source && !!target && source.id !== target.id && !!deadline;
  const body = (dryRun: boolean): RenewalCampaignCreate => ({
    name: name.trim() || undefined,
    source_season_id: sourceId,
    target_season_id: targetId,
    deadline_at: zonedInstant(deadline, tz),
    notes: notes.trim() || undefined,
    dry_run: dryRun,
  });
  const inputKey = JSON.stringify([sourceId, targetId, deadline]);
  const shownPreview = preview?.key === inputKey ? preview.result : null;

  const run = async (dryRun: boolean) => {
    setFailure(null);
    if (!valid) {
      setInvalid(true);
      return;
    }
    try {
      const result = await create.mutateAsync({
        body: body(dryRun),
        idempotencyKey: dryRun ? newIdempotencyKey() : createKey,
      });
      if (dryRun) {
        setPreview({ key: inputKey, result });
      } else if (result.campaign) {
        toast.success(t('create.done', { count: result.proposals.length }));
        onCreated(result.campaign);
        onOpenChange(false);
      }
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };

  const seasonOptions = seasons.map((s) => ({ value: s.id, label: String(s.year) }));
  return (
    <SheetForm
      onSubmit={(e?: FormEvent) => {
        e?.preventDefault();
        void run(false);
      }}
      onCancel={() => onOpenChange(false)}
      pending={create.isPending && create.variables?.body.dry_run === false}
      submitLabel={t('create.submit')}
      footerStart={
        <Button variant="outline" onClick={() => void run(true)} disabled={create.isPending}>
          {create.isPending && create.variables?.body.dry_run ? (
            <Spinner />
          ) : (
            <EyeIcon aria-hidden="true" />
          )}
          {t('create.preview')}
        </Button>
      }
    >
      <ProblemAlert problem={failure} />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label id={`${id}-source`}>{t('create.source')}</Label>
          <OptionSelect
            aria-labelledby={`${id}-source`}
            value={sourceId}
            onChange={setSourceId}
            options={seasonOptions}
            placeholder={t('create.pickSeason')}
            aria-invalid={invalid && !source}
          />
        </div>
        <div className="space-y-2">
          <Label id={`${id}-target`}>{t('create.target')}</Label>
          <OptionSelect
            aria-labelledby={`${id}-target`}
            value={targetId}
            onChange={(v) => {
              setTargetId(v);
              setDeadline(defaultDeadline(seasons.find((s) => s.id === v)));
            }}
            options={seasonOptions}
            placeholder={t('create.pickSeason')}
            aria-invalid={invalid && (!target || target.id === source?.id)}
          />
          {source && target && source.id === target.id && (
            <p className="text-sm text-destructive">{t('create.sameSeason')}</p>
          )}
        </div>
        <div className="space-y-2">
          <Label id={`${id}-deadline`}>{t('create.deadline')}</Label>
          <DatePicker
            aria-labelledby={`${id}-deadline`}
            value={deadline}
            onChange={(v) => setDeadline(v ?? '')}
            min={todayIso()}
            max={target ? addDays(target.opens_on, -1) : undefined}
          />
          <p className="text-sm text-muted-foreground">{t('create.deadlineHelp')}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${id}-name`}>
            {t('create.name')}{' '}
            <span className="font-normal text-muted-foreground">{t('optional')}</span>
          </Label>
          <Input
            id={`${id}-name`}
            value={name}
            maxLength={200}
            placeholder={target ? t('create.namePlaceholder', { year: target.year }) : undefined}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${id}-notes`}>
          {t('create.notes')}{' '}
          <span className="font-normal text-muted-foreground">{t('optional')}</span>
        </Label>
        <Textarea
          id={`${id}-notes`}
          rows={2}
          maxLength={2000}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
        />
      </div>
      {shownPreview ? (
        <CampaignPreview result={shownPreview} />
      ) : (
        <Alert role="status">
          <InfoIcon aria-hidden="true" />
          <AlertDescription>{t('create.previewHint')}</AlertDescription>
        </Alert>
      )}
    </SheetForm>
  );
}

function CampaignPreview({ result }: { result: RenewalCampaignResult }) {
  const { t } = useTranslation('renewals');
  const proposalColumns: ColumnDef<RenewalProposalPreview>[] = [
    { accessorKey: 'spot_label', header: t('columns.spot'), meta: { label: t('columns.spot') } },
    {
      accessorKey: 'customer_display_name',
      header: t('columns.customer'),
      meta: { label: t('columns.customer') },
    },
    {
      id: 'period',
      header: t('columns.period'),
      meta: { label: t('columns.period') },
      cell: ({ row }) => (
        <span className="tabular-nums">
          {formatIsoDate(row.original.starts_on)} → {formatIsoDate(row.original.ends_on)}
        </span>
      ),
    },
    {
      id: 'previous',
      header: t('columns.previous'),
      meta: { label: t('columns.previous'), align: 'right' },
      cell: ({ row }) => (
        <span className="text-muted-foreground">{formatCents(row.original.previous_amount)}</span>
      ),
    },
    {
      id: 'amount',
      header: t('columns.amount'),
      meta: { label: t('columns.amount'), align: 'right' },
      cell: ({ row }) => formatCents(row.original.total_amount),
    },
  ];
  const skippedColumns: ColumnDef<RenewalSkipped>[] = [
    {
      id: 'spot',
      header: t('columns.spot'),
      meta: { label: t('columns.spot') },
      cell: ({ row }) => row.original.spot_label ?? '—',
    },
    {
      id: 'customer',
      header: t('columns.customer'),
      meta: { label: t('columns.customer') },
      cell: ({ row }) => row.original.customer_display_name ?? '—',
    },
    {
      id: 'reason',
      header: t('columns.reason'),
      meta: { label: t('columns.reason') },
      cell: ({ row }) => t(`skip.${row.original.reason}`),
    },
  ];
  const total = result.proposals.reduce((s, p) => s + p.total_amount, 0);
  return (
    <div className="space-y-4">
      <Alert role="status" variant={result.proposals.length > 0 ? 'success' : 'warning'}>
        <InfoIcon aria-hidden="true" />
        <AlertTitle>{t('create.previewSummary', { count: result.proposals.length })}</AlertTitle>
        <AlertDescription>
          {t('create.previewDetail', {
            amount: formatCents(total),
            skipped: result.skipped.length,
          })}
        </AlertDescription>
      </Alert>
      {result.proposals.length > 0 && (
        <DataTable
          caption={t('create.previewTable')}
          columns={proposalColumns}
          data={result.proposals}
          getRowId={(p) => p.source_pass_id}
          columnToggle={false}
          pageSize={10}
        />
      )}
      {result.skipped.length > 0 && (
        <DataTable
          caption={t('create.skippedTable')}
          columns={skippedColumns}
          data={result.skipped}
          getRowId={(s) => s.source_pass_id}
          columnToggle={false}
          pageSize={10}
          toolbar={
            <span className="text-sm font-medium">
              {t('create.skippedTitle', { count: result.skipped.length })}
            </span>
          }
        />
      )}
    </div>
  );
}
