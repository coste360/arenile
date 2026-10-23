import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import {
  BackLink,
  ConfirmDialog,
  DataTable,
  DatePicker,
  EmptyState,
  OptionSelect,
  Page,
  QueryState,
  SectionCard,
  StatCard,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link, useSearch } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  CalendarRangeIcon,
  CheckCircle2Icon,
  CopyIcon,
  EyeIcon,
  HistoryIcon,
  RefreshCwIcon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { seasons as seasonsResource, type Season } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { newIdempotencyKey } from '../configuration/lib/validation';
import { describeM5bProblem } from '../renewals/problems';
import {
  useRolloverSeason,
  useSeasonRollovers,
  type SeasonRollover,
  type SeasonRolloverRequest,
  type SeasonRolloverResult,
} from './api';
import { shiftYear } from './dates';

const OPTIONS = [
  'include_rate_periods',
  'include_closure_days',
  'include_price_rules',
  'include_fixed_prices',
  'include_policies',
] as const;
type OptionKey = (typeof OPTIONS)[number];

interface Inputs {
  sourceId: string;
  targetYear: string;
  opensOn: string;
  closesOn: string;
  options: Record<OptionKey, boolean>;
}

function initialInputs(source: Season | undefined): Inputs {
  const year = source ? source.year + 1 : new Date().getFullYear() + 1;
  return {
    sourceId: source?.id ?? '',
    targetYear: String(year),
    opensOn: source ? shiftYear(source.opens_on, 1) : '',
    closesOn: source ? shiftYear(source.closes_on, 1) : '',
    options: Object.fromEntries(OPTIONS.map((o) => [o, true])) as Record<OptionKey, boolean>,
  };
}

/**
 * End-of-season wizard (spec 8.7, 10.4, 28): source season and target dates,
 * what to copy, a dry-run preview of what would be created, then apply (one
 * transaction server side, idempotent) and go on to the renewal campaign.
 * Reached from the seasons page (`?seasonId=` = source season).
 */
export function SeasonRolloverPage() {
  const { t } = useTranslation('seasonRollover');
  const establishmentId = useRequiredEstablishmentId();
  const ctx = useConfigContext();
  const search = useSearch({ strict: false }) as { seasonId?: string };
  const list = seasonsResource.useList(establishmentId);
  const seasons = [...(list.data ?? [])].sort((a, b) => b.year - a.year);
  const back = (
    <BackLink>
      <Link
        to="/e/$establishmentId/config/seasons"
        params={{ establishmentId }}
        search={(search.seasonId ? { seasonId: search.seasonId } : {}) as never}
      >
        <ArrowLeftIcon aria-hidden="true" />
        {t('back')}
      </Link>
    </BackLink>
  );

  return (
    <Page title={t('title')} description={t('description')} eyebrow={back}>
      {list.isPending || list.error ? (
        <QueryState
          isLoading={list.isPending}
          error={list.error}
          onRetry={() => void list.refetch()}
        />
      ) : seasons.length === 0 ? (
        <EmptyState icon={CalendarRangeIcon} title={t('noSeasons')} />
      ) : (
        <Wizard
          establishmentId={establishmentId}
          seasons={seasons}
          initialSource={seasons.find((s) => s.id === search.seasonId) ?? seasons[0]}
          canConfigure={ctx.canConfigure}
        />
      )}
      <History establishmentId={establishmentId} seasons={seasons} />
    </Page>
  );
}

function Wizard({
  establishmentId,
  seasons,
  initialSource,
  canConfigure,
}: {
  establishmentId: string;
  seasons: Season[];
  initialSource: Season | undefined;
  canConfigure: boolean;
}) {
  const { t } = useTranslation('seasonRollover');
  const id = useId();
  const rollover = useRolloverSeason(establishmentId);
  const [inputs, setInputs] = useState<Inputs>(() => initialInputs(initialSource));
  const [preview, setPreview] = useState<{ key: string; result: SeasonRolloverResult } | null>(
    null,
  );
  const [applied, setApplied] = useState<SeasonRolloverResult | null>(null);
  const [applyKey, setApplyKey] = useState<{ key: string; idem: string } | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const [confirming, setConfirming] = useState(false);
  const source = seasons.find((s) => s.id === inputs.sourceId);
  const year = Number(inputs.targetYear);
  const yearValid = Number.isInteger(year) && year >= 2000 && year <= 2100;
  const datesValid = !!inputs.opensOn && !!inputs.closesOn && inputs.opensOn <= inputs.closesOn;
  const valid = !!source && yearValid && datesValid;
  const inputKey = JSON.stringify(inputs);
  const shown = preview?.key === inputKey ? preview.result : null;

  const changeSource = (sourceId: string) => {
    const s = seasons.find((x) => x.id === sourceId);
    setInputs((prev) => ({ ...initialInputs(s), options: prev.options }));
  };
  const changeYear = (value: string) => {
    const y = Number(value);
    setInputs((prev) => {
      if (!source || !Number.isInteger(y) || y < 2000 || y > 2100)
        return { ...prev, targetYear: value };
      const delta = y - source.year;
      return {
        ...prev,
        targetYear: value,
        opensOn: shiftYear(source.opens_on, delta),
        closesOn: shiftYear(source.closes_on, delta),
      };
    });
  };

  const request = (dryRun: boolean): SeasonRolloverRequest => ({
    target_year: year,
    opens_on: inputs.opensOn,
    closes_on: inputs.closesOn,
    ...inputs.options,
    dry_run: dryRun,
  });

  const runPreview = async () => {
    if (!valid || !source) return;
    setFailure(null);
    try {
      const result = await rollover.mutateAsync({
        seasonId: source.id,
        body: request(true),
        idempotencyKey: newIdempotencyKey(),
      });
      setPreview({ key: inputKey, result });
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };

  const apply = async () => {
    if (!source) return;
    const idem = applyKey?.key === inputKey ? applyKey.idem : newIdempotencyKey();
    setApplyKey({ key: inputKey, idem });
    const result = await rollover.mutateAsync({
      seasonId: source.id,
      body: request(false),
      idempotencyKey: idem,
    });
    setApplied(result);
  };

  if (applied) return <Applied establishmentId={establishmentId} result={applied} />;

  return (
    <>
      <ProblemAlert problem={failure} />
      <SectionCard
        title={t('step.season')}
        description={t('step.seasonHelp')}
        icon={CalendarRangeIcon}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label id={`${id}-source`}>{t('fields.source')}</Label>
            <OptionSelect
              aria-labelledby={`${id}-source`}
              value={inputs.sourceId}
              onChange={changeSource}
              options={seasons.map((s) => ({
                value: s.id,
                label: `${s.year} · ${formatIsoDate(s.opens_on)} → ${formatIsoDate(s.closes_on)}`,
              }))}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-year`}>{t('fields.targetYear')}</Label>
            <Input
              id={`${id}-year`}
              inputMode="numeric"
              value={inputs.targetYear}
              aria-invalid={!yearValid}
              onChange={(e) => changeYear(e.target.value)}
            />
            {!yearValid && <p className="text-sm text-destructive">{t('fields.yearInvalid')}</p>}
          </div>
          <div className="space-y-2">
            <Label id={`${id}-opens`}>{t('fields.opensOn')}</Label>
            <DatePicker
              aria-labelledby={`${id}-opens`}
              value={inputs.opensOn}
              onChange={(v) => setInputs((p) => ({ ...p, opensOn: v ?? '' }))}
            />
          </div>
          <div className="space-y-2">
            <Label id={`${id}-closes`}>{t('fields.closesOn')}</Label>
            <DatePicker
              aria-labelledby={`${id}-closes`}
              value={inputs.closesOn}
              onChange={(v) => setInputs((p) => ({ ...p, closesOn: v ?? '' }))}
              min={inputs.opensOn || undefined}
              aria-invalid={!datesValid}
            />
            {inputs.opensOn && inputs.closesOn && !datesValid && (
              <p className="text-sm text-destructive">{t('fields.datesInvalid')}</p>
            )}
          </div>
        </div>
      </SectionCard>

      <SectionCard
        title={t('step.copy')}
        description={t('step.copyHelp')}
        icon={CopyIcon}
        footer={
          <Button onClick={() => void runPreview()} disabled={!valid || rollover.isPending}>
            {rollover.isPending && rollover.variables?.body.dry_run ? (
              <Spinner />
            ) : (
              <EyeIcon aria-hidden="true" />
            )}
            {t('preview')}
          </Button>
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          {OPTIONS.map((o) => (
            <div key={o} className="flex items-start gap-3">
              <Switch
                id={`${id}-${o}`}
                checked={inputs.options[o]}
                onCheckedChange={(v) =>
                  setInputs((p) => ({ ...p, options: { ...p.options, [o]: v } }))
                }
              />
              <div className="min-w-0 space-y-1">
                <Label htmlFor={`${id}-${o}`}>{t(`options.${o}.label`)}</Label>
                <p className="text-sm text-muted-foreground">{t(`options.${o}.help`)}</p>
              </div>
            </div>
          ))}
        </div>
      </SectionCard>

      {shown && (
        <SectionCard
          title={t('step.preview', { year: shown.target_year })}
          description={t('step.previewHelp', {
            from: formatIsoDate(shown.opens_on),
            to: formatIsoDate(shown.closes_on),
          })}
          icon={EyeIcon}
          footer={
            canConfigure && (
              <Button onClick={() => setConfirming(true)} disabled={rollover.isPending}>
                <RefreshCwIcon aria-hidden="true" />
                {shown.already_applied ? t('openApplied') : t('apply', { year: shown.target_year })}
              </Button>
            )
          }
        >
          <div className="space-y-4">
            {shown.already_applied && (
              <Alert role="status">
                <CheckCircle2Icon aria-hidden="true" />
                <AlertTitle>{t('alreadyApplied', { year: shown.target_year })}</AlertTitle>
              </Alert>
            )}
            <Counts result={shown} />
            {shown.warnings.length > 0 && (
              <Alert variant="warning" role="status">
                <TriangleAlertIcon aria-hidden="true" />
                <AlertTitle>{t('warnings')}</AlertTitle>
                <AlertDescription>
                  <ul className="list-inside list-disc">
                    {shown.warnings.map((w, i) => (
                      <li key={`${w.code}-${w.date ?? ''}-${i}`}>
                        {w.code === 'season_copy'
                          ? w.text
                          : t(`warning.${w.code}`, {
                              date: w.date ? formatIsoDate(w.date) : '',
                              target: w.target_date ? formatIsoDate(w.target_date) : '',
                            })}
                      </li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}
            {!canConfigure && <p className="text-sm text-muted-foreground">{t('readOnly')}</p>}
          </div>
        </SectionCard>
      )}

      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('confirm.title', { year })}
        description={t('confirm.description', { year })}
        confirmLabel={t('confirm.submit')}
        errorText={(err) => describeM5bProblem(err, t).title}
        onConfirm={apply}
      />
    </>
  );
}

function Counts({ result }: { result: SeasonRolloverResult }) {
  const { t } = useTranslation('seasonRollover');
  const c = result.counts;
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
      <StatCard label={t('counts.rate_periods')} value={c.rate_periods} />
      <StatCard label={t('counts.closure_days')} value={c.closure_days} />
      <StatCard label={t('counts.price_rules')} value={c.price_rules} />
      <StatCard label={t('counts.fixed_prices')} value={c.fixed_prices} />
      <StatCard label={t('counts.policies')} value={c.policies} />
      <StatCard
        label={t('counts.season_passes')}
        value={result.season_passes ?? 0}
        hint={t('counts.seasonPassesHint')}
      />
    </div>
  );
}

function Applied({
  establishmentId,
  result,
}: {
  establishmentId: string;
  result: SeasonRolloverResult;
}) {
  const { t } = useTranslation('seasonRollover');
  return (
    <SectionCard
      title={
        result.already_applied
          ? t('alreadyApplied', { year: result.target_year })
          : t('done.title', { year: result.target_year })
      }
      description={t('done.description')}
      icon={CheckCircle2Icon}
      footer={
        <>
          {result.season && (
            <Button variant="outline" asChild>
              <Link
                to="/e/$establishmentId/config/seasons"
                params={{ establishmentId }}
                search={{ seasonId: result.season.id } as never}
              >
                {t('done.openSeason')}
              </Link>
            </Button>
          )}
          <Button asChild>
            <Link
              to="/e/$establishmentId/rinnovi"
              params={{ establishmentId }}
              search={
                {
                  source: result.source_season_id,
                  target: result.season?.id,
                } as never
              }
            >
              <RefreshCwIcon aria-hidden="true" />
              {t('done.renewals', { count: result.season_passes ?? 0 })}
            </Link>
          </Button>
        </>
      }
    >
      <Counts result={result} />
    </SectionCard>
  );
}

function History({ establishmentId, seasons }: { establishmentId: string; seasons: Season[] }) {
  const { t } = useTranslation('seasonRollover');
  const fmt = useInstantFormat();
  const list = useSeasonRollovers(establishmentId);
  const yearOf = (seasonId: string) => seasons.find((s) => s.id === seasonId)?.year ?? '—';
  const columns: ColumnDef<SeasonRollover>[] = [
    {
      id: 'seasons',
      header: t('history.seasons'),
      meta: { label: t('history.seasons') },
      cell: ({ row }) => (
        <span className="font-medium tabular-nums">
          {yearOf(row.original.source_season_id)} → {row.original.target_year}
        </span>
      ),
    },
    {
      id: 'copied',
      header: t('history.copied'),
      meta: { label: t('history.copied') },
      cell: ({ row: { original: r } }) =>
        t('history.summary', {
          periods: r.counts.rate_periods,
          rules: r.counts.price_rules,
          prices: r.counts.fixed_prices,
        }),
    },
    {
      id: 'when',
      header: t('history.when'),
      meta: { label: t('history.when') },
      cell: ({ row }) => (
        <span className="tabular-nums">{fmt.dateTime(row.original.created_at)}</span>
      ),
    },
  ];
  if (list.isSuccess && list.data.items.length === 0) return null;
  return (
    <SectionCard flush title={t('history.title')} icon={HistoryIcon}>
      <DataTable
        embedded
        caption={t('history.title')}
        columns={columns}
        data={list.data?.items ?? []}
        loading={list.isPending}
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        getRowId={(r) => r.id}
        columnToggle={false}
        pageSize={0}
      />
    </SectionCard>
  );
}
