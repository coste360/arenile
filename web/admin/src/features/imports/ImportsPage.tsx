import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import {
  ConfirmDialog,
  DataTable,
  EmptyState,
  OptionSelect,
  SectionCard,
  SegmentedControl,
  StatCard,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import {
  CheckCircle2Icon,
  DownloadIcon,
  FileUpIcon,
  HistoryIcon,
  ListChecksIcon,
  SearchCheckIcon,
  Trash2Icon,
  TriangleAlertIcon,
  UploadIcon,
} from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { packs as packsResource, seasons as seasonsResource } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { localize } from '../configuration/lib/i18nText';
import {
  templateUrl,
  useApplyImport,
  useDiscardImport,
  useFetchImport,
  useImportJobs,
  usePreviewImport,
  useUploadImport,
  type ImportIssue,
  type ImportJob,
  type ImportKind,
  type ImportOptions,
} from './api';

type JobStatus = ImportJob['status'];

const STATUS_TONE: Record<JobStatus, StatusTone> = {
  uploaded: 'neutral',
  validated: 'info',
  applying: 'info',
  applied: 'success',
  failed: 'danger',
  discarded: 'neutral',
};

/** Options sent with the preview; the server applies its defaults to the rest. */
function defaultOptions(kind: ImportKind): ImportOptions {
  return kind === 'season_passes'
    ? { match_by: 'email', update_existing: false, create_missing_customers: true }
    : { match_by: 'email', update_existing: false, marketing_consent_default: false };
}

/**
 * Configurazione → Importa dati (ADR 0019 F): upload a CSV exported from
 * another program, preview it (dry run, validation report by row and column),
 * then apply it all or nothing. Re-applying the same file is refused by the
 * server; rows matched by email or phone are updated or skipped.
 */
export function ImportsPage() {
  const { t } = useTranslation('imports');
  const ctx = useConfigContext();
  const [job, setJob] = useState<ImportJob | null>(null);
  const readOnly = !!ctx.establishment && !ctx.canConfigure;
  return (
    <ConfigPage title={t('page.title')} description={t('page.description')} readOnly={readOnly}>
      <div className="grid gap-6">
        {job && job.status !== 'applied' && job.status !== 'discarded' ? (
          <ReviewSection
            establishmentId={ctx.establishmentId}
            job={job}
            onChange={setJob}
            readOnly={readOnly}
          />
        ) : (
          <UploadSection
            establishmentId={ctx.establishmentId}
            readOnly={readOnly}
            done={job?.status === 'applied' ? job : null}
            onUploaded={setJob}
          />
        )}
        <HistorySection establishmentId={ctx.establishmentId} onOpen={setJob} />
      </div>
    </ConfigPage>
  );
}

function UploadSection({
  establishmentId,
  readOnly,
  done,
  onUploaded,
}: {
  establishmentId: string;
  readOnly: boolean;
  done: ImportJob | null;
  onUploaded: (j: ImportJob) => void;
}) {
  const { t } = useTranslation('imports');
  const problemText = useProblemText();
  const upload = useUploadImport(establishmentId);
  const [kind, setKind] = useState<ImportKind>('customers');
  const [file, setFile] = useState<File | null>(null);
  const fileId = useId();

  const submit = () => {
    if (!file) return;
    upload.mutate(
      { kind, file },
      {
        onSuccess: (j) => {
          setFile(null);
          onUploaded(j);
        },
      },
    );
  };

  return (
    <SectionCard
      icon={FileUpIcon}
      title={t('upload.title')}
      description={t('upload.description')}
      footer={
        <Button disabled={readOnly || !file || upload.isPending} onClick={submit}>
          {upload.isPending ? <Spinner /> : <UploadIcon aria-hidden="true" />}
          {t('upload.submit')}
        </Button>
      }
    >
      <div className="grid gap-4">
        {done?.report && (
          <Alert variant="success" role="status">
            <CheckCircle2Icon aria-hidden="true" />
            <AlertTitle>{t('applied.title')}</AlertTitle>
            <AlertDescription>
              {t('applied.description', {
                created: done.report.created ?? 0,
                updated: done.report.updated ?? 0,
                skipped: done.report.skipped ?? 0,
              })}
            </AlertDescription>
          </Alert>
        )}
        <SegmentedControl
          label={t('upload.kind')}
          value={kind}
          onValueChange={(v) => setKind(v as ImportKind)}
          options={[
            { value: 'customers', label: t('kind.customers') },
            { value: 'season_passes', label: t('kind.season_passes') },
          ]}
        />
        <p className="text-sm text-muted-foreground">{t(`upload.columns.${kind}`)}</p>
        <div>
          <Button variant="outline" asChild>
            <a href={templateUrl(establishmentId, kind)} download={`arenile-${kind}.csv`}>
              <DownloadIcon aria-hidden="true" />
              {t('upload.template')}
            </a>
          </Button>
        </div>
        <Field>
          <FieldLabel htmlFor={fileId}>{t('upload.file')}</FieldLabel>
          <Input
            id={fileId}
            type="file"
            accept=".csv,text/csv"
            disabled={readOnly}
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
          <FieldDescription>{t('upload.fileHint')}</FieldDescription>
        </Field>
        {upload.error && (
          <Alert variant="destructive">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(upload.error)}</AlertDescription>
          </Alert>
        )}
      </div>
    </SectionCard>
  );
}

function ReviewSection({
  establishmentId,
  job,
  onChange,
  readOnly,
}: {
  establishmentId: string;
  job: ImportJob;
  onChange: (j: ImportJob | null) => void;
  readOnly: boolean;
}) {
  const { t, i18n } = useTranslation('imports');
  const problemText = useProblemText();
  const preview = usePreviewImport(establishmentId);
  const apply = useApplyImport(establishmentId);
  const discard = useDiscardImport(establishmentId);
  const fetchJob = useFetchImport(establishmentId);
  const seasonList = seasonsResource.useList(establishmentId);
  const packList = packsResource.useList(establishmentId);
  const [options, setOptions] = useState<ImportOptions>(() =>
    job.report ? job.options : defaultOptions(job.kind),
  );
  const isPasses = job.kind === 'season_passes';
  const seasons = [...(seasonList.data ?? [])].sort((a, b) => b.year - a.year);
  const packs = (packList.data ?? [])
    .filter(
      (p) => p.active && (!p.allowed_formulas?.length || p.allowed_formulas.includes('season')),
    )
    .sort((a, b) => a.sort_order - b.sort_order);
  const report = job.status === 'validated' || job.status === 'failed' ? job.report : undefined;
  const canApply =
    job.status === 'validated' && !!report && report.invalid === 0 && report.valid > 0;
  const set = (patch: Partial<ImportOptions>) => setOptions((o) => ({ ...o, ...patch }));

  const runPreview = () =>
    preview.mutate(
      { job, options },
      { onSuccess: onChange, onError: (e) => toast.error(problemText(e)) },
    );

  const runApply = async () => {
    try {
      const done = await apply.mutateAsync(job);
      toast.success(t('applied.title'));
      onChange(done);
    } catch (e) {
      // A refused apply rolls back everything: reload the report it stored.
      onChange(await fetchJob(job.id).catch(() => job));
      throw e;
    }
  };

  return (
    <SectionCard
      icon={ListChecksIcon}
      title={t('review.title', { file: job.filename || t('review.noName') })}
      description={t(`kind.${job.kind}`)}
      actions={
        <StatusBadge tone={STATUS_TONE[job.status]}>{t(`status.${job.status}`)}</StatusBadge>
      }
      footer={
        <>
          <ConfirmDialog
            trigger={
              <Button variant="outline" disabled={readOnly || discard.isPending}>
                <Trash2Icon aria-hidden="true" />
                {t('review.discard')}
              </Button>
            }
            title={t('review.discardTitle')}
            description={t('review.discardDescription')}
            confirmLabel={t('review.discard')}
            destructive
            onConfirm={async () => {
              await discard.mutateAsync(job);
              onChange(null);
            }}
            errorText={problemText}
          />
          <Button
            variant={canApply ? 'outline' : 'default'}
            disabled={readOnly || preview.isPending}
            onClick={runPreview}
          >
            {preview.isPending ? <Spinner /> : <SearchCheckIcon aria-hidden="true" />}
            {report ? t('review.previewAgain') : t('review.preview')}
          </Button>
          <ConfirmDialog
            trigger={
              <Button disabled={readOnly || !canApply}>
                <CheckCircle2Icon aria-hidden="true" />
                {t('review.apply')}
              </Button>
            }
            title={t('review.applyTitle')}
            description={t('review.applyDescription', {
              create: report?.to_create ?? 0,
              update: report?.to_update ?? 0,
              skip: report?.to_skip ?? 0,
            })}
            confirmLabel={t('review.apply')}
            onConfirm={runApply}
            errorText={problemText}
          />
        </>
      }
    >
      <div className="grid gap-6">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <span className="text-sm font-medium">{t('options.matchBy')}</span>
            <SegmentedControl
              label={t('options.matchBy')}
              value={options.match_by ?? 'email'}
              onValueChange={(v) => set({ match_by: v as ImportOptions['match_by'] })}
              options={[
                { value: 'email', label: t('options.matchEmail') },
                { value: 'phone', label: t('options.matchPhone') },
              ]}
            />
          </div>
          {isPasses && (
            <>
              <div className="grid gap-2">
                <span className="text-sm font-medium">{t('options.season')}</span>
                <OptionSelect
                  value={options.season_id ?? ''}
                  onChange={(v) => set({ season_id: v || undefined })}
                  placeholder={t('options.seasonPlaceholder')}
                  aria-label={t('options.season')}
                  options={seasons.map((s) => ({ value: s.id, label: String(s.year) }))}
                />
              </div>
              <div className="grid gap-2">
                <span className="text-sm font-medium">{t('options.pack')}</span>
                <OptionSelect
                  value={options.pack_id ?? ''}
                  onChange={(v) => set({ pack_id: v || undefined })}
                  placeholder={t('options.packPlaceholder')}
                  aria-label={t('options.pack')}
                  options={packs.map((p) => ({
                    value: p.id,
                    label: localize(p.name, i18n.language, 'it') || p.code,
                  }))}
                />
              </div>
            </>
          )}
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <OptionSwitch
            id="import-update"
            label={t('options.updateExisting')}
            hint={t('options.updateExistingHint')}
            checked={!!options.update_existing}
            onChange={(v) => set({ update_existing: v })}
            disabled={readOnly}
          />
          {isPasses ? (
            <OptionSwitch
              id="import-create-customers"
              label={t('options.createCustomers')}
              hint={t('options.createCustomersHint')}
              checked={options.create_missing_customers !== false}
              onChange={(v) => set({ create_missing_customers: v })}
              disabled={readOnly}
            />
          ) : (
            <OptionSwitch
              id="import-consent"
              label={t('options.consentDefault')}
              hint={t('options.consentDefaultHint')}
              checked={!!options.marketing_consent_default}
              onChange={(v) => set({ marketing_consent_default: v })}
              disabled={readOnly}
            />
          )}
        </div>
        {preview.error && (
          <Alert variant="destructive">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(preview.error)}</AlertDescription>
          </Alert>
        )}
        {job.status === 'failed' && (
          <Alert variant="destructive">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertTitle>{t('failed.title')}</AlertTitle>
            <AlertDescription>{t('failed.description')}</AlertDescription>
          </Alert>
        )}
        {report ? (
          <ReportView report={report} />
        ) : (
          <p className="text-sm text-muted-foreground">{t('review.previewHint')}</p>
        )}
      </div>
    </SectionCard>
  );
}

function OptionSwitch({
  id,
  label,
  hint,
  checked,
  onChange,
  disabled,
}: {
  id: string;
  label: string;
  hint: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled: boolean;
}) {
  return (
    <Field
      orientation="horizontal"
      data-disabled={disabled || undefined}
      className="rounded-lg border p-3"
    >
      <FieldContent>
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        <FieldDescription id={`${id}-hint`}>{hint}</FieldDescription>
      </FieldContent>
      <Switch
        id={id}
        aria-describedby={`${id}-hint`}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
    </Field>
  );
}

function ReportView({ report }: { report: NonNullable<ImportJob['report']> }) {
  const { t } = useTranslation('imports');
  const issueColumns: ColumnDef<ImportIssue>[] = [
    {
      accessorKey: 'row',
      meta: { label: t('report.row'), align: 'right' },
      header: t('report.row'),
    },
    {
      id: 'column',
      accessorFn: (i) => i.column ?? '',
      meta: { label: t('report.column') },
      header: t('report.column'),
      cell: ({ row }) => row.original.column || '—',
    },
    {
      id: 'message',
      accessorFn: (i) => i.message ?? i.code,
      meta: { label: t('report.problem') },
      header: t('report.problem'),
      cell: ({ row }) => (
        <span className="break-words">
          {row.original.message ||
            t(`codes.${row.original.code}`, { defaultValue: row.original.code })}
        </span>
      ),
    },
  ];
  return (
    <div className="grid gap-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label={t('report.rows')} value={report.rows} />
        <StatCard
          label={t('report.invalid')}
          value={report.invalid}
          tone={report.invalid > 0 ? 'danger' : 'default'}
        />
        <StatCard label={t('report.toCreate')} value={report.to_create} tone="success" />
        <StatCard
          label={t('report.toUpdateSkip')}
          value={`${report.to_update} / ${report.to_skip}`}
        />
      </div>
      {report.invalid > 0 && (
        <Alert variant="warning">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription>{t('report.fixAndRetry')}</AlertDescription>
        </Alert>
      )}
      {report.errors.length > 0 && (
        <div className="min-w-0 space-y-2">
          <p className="font-semibold">{t('report.errors')}</p>
          <DataTable
            caption={t('report.errors')}
            columns={issueColumns}
            data={report.errors}
            getRowId={(i) => `e-${i.row}-${i.column ?? ''}-${i.code}`}
            columnToggle={false}
          />
        </div>
      )}
      {report.warnings.length > 0 && (
        <div className="min-w-0 space-y-2">
          <p className="font-semibold">{t('report.warnings')}</p>
          <DataTable
            caption={t('report.warnings')}
            columns={issueColumns}
            data={report.warnings}
            getRowId={(i) => `w-${i.row}-${i.column ?? ''}-${i.code}`}
            columnToggle={false}
          />
        </div>
      )}
    </div>
  );
}

function HistorySection({
  establishmentId,
  onOpen,
}: {
  establishmentId: string;
  onOpen: (j: ImportJob) => void;
}) {
  const { t } = useTranslation('imports');
  const fmt = useInstantFormat();
  const q = useImportJobs(establishmentId);
  const rows = q.data?.pages.flatMap((p) => p.items) ?? [];
  const columns: ColumnDef<ImportJob>[] = [
    {
      id: 'file',
      accessorFn: (j) => j.filename,
      meta: { label: t('history.file') },
      header: t('history.file'),
      cell: ({ row }) => (
        <span className="flex flex-col">
          <span className="font-medium break-all">
            {row.original.filename || t('review.noName')}
          </span>
          <span className="text-xs text-muted-foreground">{t(`kind.${row.original.kind}`)}</span>
        </span>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('history.status') },
      header: t('history.status'),
      cell: ({ row }) => (
        <StatusBadge tone={STATUS_TONE[row.original.status]}>
          {t(`status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'result',
      accessorFn: (j) => j.report?.rows ?? 0,
      meta: { label: t('history.result') },
      header: t('history.result'),
      cell: ({ row }) => {
        const r = row.original.report;
        if (!r) return '—';
        return row.original.status === 'applied'
          ? t('history.appliedResult', {
              created: r.created ?? 0,
              updated: r.updated ?? 0,
              skipped: r.skipped ?? 0,
            })
          : t('history.previewResult', { rows: r.rows, invalid: r.invalid });
      },
    },
    {
      id: 'when',
      accessorFn: (j) => j.updated_at,
      meta: { label: t('history.when') },
      header: t('history.when'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap text-muted-foreground">
          {fmt.dateTime(row.original.applied_at ?? row.original.updated_at)}
        </span>
      ),
    },
  ];
  const open = (j: ImportJob) => {
    if (j.status === 'uploaded' || j.status === 'validated' || j.status === 'failed') onOpen(j);
  };
  return (
    <SectionCard icon={HistoryIcon} title={t('history.title')} flush>
      <DataTable
        embedded
        caption={t('history.title')}
        columns={columns}
        data={rows}
        loading={q.isPending}
        getRowId={(j) => j.id}
        columnToggle={false}
        onRowClick={open}
        getRowLabel={(j) => t('history.openAria', { file: j.filename || t('review.noName') })}
        loadMore={{
          hasMore: !!q.hasNextPage,
          loading: q.isFetchingNextPage,
          onLoadMore: () => void q.fetchNextPage(),
        }}
        empty={<EmptyState title={t('history.empty')} bordered={false} />}
      />
    </SectionCard>
  );
}
