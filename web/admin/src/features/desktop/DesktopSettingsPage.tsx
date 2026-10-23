import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import {
  type ColumnDef,
  ConfirmDialog,
  DataTable,
  EmptyState,
  FieldRow,
  FormSection,
  Page,
  SectionCard,
  QueryState,
  StatusBadge,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  ArchiveIcon,
  DatabaseBackupIcon,
  HistoryIcon,
  RotateCcwIcon,
  WifiIcon,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { z } from 'zod';
import { useInstantFormat } from '../../establishment';
import { useInstanceMeta } from '../../lib/meta';
import {
  type DesktopBackup,
  type DesktopSettings,
  useCreateDesktopBackup,
  useDesktopBackups,
  useDesktopSettings,
  useReloadWhenServerIsBack,
  useRestoreDesktopBackup,
  useUpdateDesktopSettings,
} from './api';
import { formatBytes } from './lib';

const settingsSchema = z.object({
  backup_dir: z.string().trim().min(1, 'desktop:validation.backupDir'),
  backup_retention: z
    .number()
    .int('desktop:validation.retention')
    .min(1, 'desktop:validation.retention')
    .max(365, 'desktop:validation.retention'),
  backup_daily: z.boolean(),
  backup_on_exit: z.boolean(),
  update_check: z.boolean(),
  autostart: z.boolean(),
  keep_running_in_tray: z.boolean(),
  lan_enabled: z.boolean(),
  lan_port: z
    .number()
    .int('desktop:validation.port')
    .min(1024, 'desktop:validation.port')
    .max(65535, 'desktop:validation.port'),
});

type SettingsValues = z.infer<typeof settingsSchema>;

const REASON_TONE: Record<DesktopBackup['reason'], StatusTone> = {
  manual: 'info',
  daily: 'neutral',
  exit: 'neutral',
  pre_restore: 'warning',
};

function toValues(s: DesktopSettings): SettingsValues {
  return {
    ...s,
    lan_port: s.lan_port ?? 8443,
    keep_running_in_tray: s.keep_running_in_tray ?? false,
  };
}

/**
 * Configurazione → App desktop (desktop edition only, owner; spec 29.1):
 * backup folder (any absolute path, e.g. a USB stick: chosen here instead
 * of a native dialog), retention, daily and on-exit backups, "Backup ora",
 * the list of backups with "Ripristina" (the launcher restarts the server
 * on the restored file and the page reloads), start at login, what closing
 * the app window does (keep running in the tray), the opt-in update check
 * and the LAN mode.
 */
export function DesktopSettingsPage() {
  const { t } = useTranslation('desktop');
  const settings = useDesktopSettings();

  return (
    <Page title={t('settings.title')} description={t('settings.description')}>
      {settings.data ? (
        <SettingsForm initial={settings.data} />
      ) : (
        <QueryState
          isLoading={settings.isPending}
          error={settings.error}
          onRetry={() => void settings.refetch()}
        />
      )}
      <BackupsSection />
    </Page>
  );
}

function SettingsForm({ initial }: { initial: DesktopSettings }) {
  const { t } = useTranslation('desktop');
  const update = useUpdateDesktopSettings();
  const problemText = useProblemText();
  const meta = useInstanceMeta();
  const form = useForm<SettingsValues>({
    resolver: zodResolver(settingsSchema),
    defaultValues: toValues(initial),
  });
  useEffect(() => form.reset(toValues(initial)), [initial, form]);

  const onSubmit = form.handleSubmit((values) => {
    update.mutate(values, {
      onSuccess: () => toast.success(t('settings.saved')),
    });
  });
  const lanOn = useWatch({ control: form.control, name: 'lan_enabled' });
  const desktop = meta.data?.desktop;

  return (
    <Form {...form}>
      <form onSubmit={onSubmit} noValidate aria-label={t('settings.title')} className="space-y-6">
        {update.error && (
          <Alert variant="destructive">
            <AlertTitle>{t('settings.saveFailed')}</AlertTitle>
            <AlertDescription>{problemText(update.error)}</AlertDescription>
          </Alert>
        )}
        <FormSection title={t('backup.title')} description={t('backup.description')}>
          <FieldRow
            control={form.control}
            name="backup_dir"
            label={t('backup.dir')}
            description={t('backup.dirHint')}
          >
            {(field) => <Input spellCheck={false} autoComplete="off" {...field} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="backup_retention"
            label={t('backup.retention')}
            description={t('backup.retentionHint')}
          >
            {(field) => (
              <Input
                type="number"
                inputMode="numeric"
                min={1}
                max={365}
                name={field.name}
                ref={field.ref}
                onBlur={field.onBlur}
                value={field.value}
                onChange={(e) => field.onChange(Number(e.target.value) || 0)}
              />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="backup_daily" label={t('backup.daily')}>
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
          <FieldRow control={form.control} name="backup_on_exit" label={t('backup.onExit')}>
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
        </FormSection>
        <FormSection title={t('system.title')} description={t('system.description')}>
          <FieldRow
            control={form.control}
            name="autostart"
            label={t('system.autostart')}
            description={t('system.autostartHint')}
          >
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="keep_running_in_tray"
            label={t('system.keepRunningInTray')}
            description={t('system.keepRunningInTrayHint')}
          >
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="update_check"
            label={t('system.updateCheck')}
            description={t('system.updateCheckHint')}
          >
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
        </FormSection>
        <FormSection
          title={t('lan.title')}
          description={t('lan.description')}
          footer={
            <Button type="submit" disabled={update.isPending}>
              {update.isPending && <Spinner />}
              {t('settings.save')}
            </Button>
          }
        >
          <FieldRow control={form.control} name="lan_enabled" label={t('lan.enabled')}>
            {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
          </FieldRow>
          {lanOn && (
            <FieldRow
              control={form.control}
              name="lan_port"
              label={t('lan.port')}
              description={t('lan.portHint')}
            >
              {(field) => (
                <Input
                  type="number"
                  inputMode="numeric"
                  min={1024}
                  max={65535}
                  name={field.name}
                  ref={field.ref}
                  onBlur={field.onBlur}
                  value={field.value}
                  onChange={(e) => field.onChange(Number(e.target.value) || 0)}
                />
              )}
            </FieldRow>
          )}
          {desktop?.mode === 'lan' && desktop.lan_url && (
            <Alert role="note">
              <WifiIcon aria-hidden="true" />
              <AlertTitle>{t('lan.activeTitle')}</AlertTitle>
              <AlertDescription>
                <p>{t('lan.address', { url: desktop.lan_url })}</p>
                {desktop.lan_cert_fingerprint && (
                  <p className="font-mono text-xs break-all">
                    {t('lan.fingerprint', { fingerprint: desktop.lan_cert_fingerprint })}
                  </p>
                )}
              </AlertDescription>
            </Alert>
          )}
        </FormSection>
      </form>
    </Form>
  );
}

function BackupsSection() {
  const { t, i18n } = useTranslation('desktop');
  const backups = useDesktopBackups();
  const create = useCreateDesktopBackup();
  const restore = useRestoreDesktopBackup();
  const problemText = useProblemText();
  const fmt = useInstantFormat();
  const [restoring, setRestoring] = useState<DesktopBackup | null>(null);
  const [restarted, setRestarted] = useState(false);
  useReloadWhenServerIsBack(restarted);

  const columns: ColumnDef<DesktopBackup>[] = [
    {
      accessorKey: 'created_at',
      meta: { label: t('backups.date') },
      header: t('backups.date'),
      cell: ({ row }) => (
        <span className="flex min-w-0 flex-col">
          <span className="whitespace-nowrap">{fmt.dateTime(row.original.created_at)}</span>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {row.original.name}
          </span>
        </span>
      ),
    },
    {
      accessorKey: 'reason',
      meta: { label: t('backups.reason') },
      header: t('backups.reason'),
      cell: ({ row }) => (
        <StatusBadge tone={REASON_TONE[row.original.reason]}>
          {t(`backups.reasons.${row.original.reason}`)}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'size_bytes',
      meta: { label: t('backups.size'), align: 'right' },
      header: t('backups.size'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatBytes(row.original.size_bytes, i18n.language)}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('backups.actions')}</span>,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <Button
            variant="ghost"
            size="sm"
            aria-label={t('restore.aria', { name: row.original.name })}
            onClick={() => setRestoring(row.original)}
            disabled={restarted}
          >
            <RotateCcwIcon aria-hidden="true" />
            <span className="hidden sm:inline">{t('restore.action')}</span>
          </Button>
        </div>
      ),
    },
  ];

  return (
    <section aria-labelledby="desktop-backups">
      <SectionCard
        titleId="desktop-backups"
        title={t('backups.title')}
        description={
          backups.data && (
            <span className="break-all">
              {t('backups.directory', { dir: backups.data.directory })}
            </span>
          )
        }
        actions={
          <Button
            variant="outline"
            disabled={create.isPending || restarted}
            onClick={() =>
              create.mutate(undefined, {
                onSuccess: (b) => toast.success(t('backups.done', { name: b.name })),
                onError: (err) => toast.error(problemText(err)),
              })
            }
          >
            {create.isPending ? <Spinner /> : <DatabaseBackupIcon aria-hidden="true" />}
            {t('backups.now')}
          </Button>
        }
        flush
      >
        <div className="space-y-4">
          {restarted && (
            <div className="px-6">
              <Alert role="status">
                <HistoryIcon aria-hidden="true" />
                <AlertTitle>{t('restore.inProgressTitle')}</AlertTitle>
                <AlertDescription>{t('restore.inProgress')}</AlertDescription>
              </Alert>
            </div>
          )}
          <DataTable
            embedded
            columnToggle={false}
            error={
              backups.error ? (
                <QueryState
                  isLoading={false}
                  error={backups.error}
                  onRetry={() => void backups.refetch()}
                />
              ) : undefined
            }
            caption={t('backups.title')}
            columns={columns}
            data={backups.data?.items ?? []}
            loading={backups.isPending}
            getRowId={(b) => b.name}
            empty={
              <EmptyState
                icon={ArchiveIcon}
                title={t('backups.empty.title')}
                description={t('backups.empty.description')}
                bordered={false}
              />
            }
          />
        </div>
      </SectionCard>
      <ConfirmDialog
        open={restoring !== null}
        onOpenChange={(open) => {
          if (!open) setRestoring(null);
        }}
        title={t('restore.title')}
        description={
          restoring ? t('restore.message', { date: fmt.dateTime(restoring.created_at) }) : undefined
        }
        confirmLabel={t('restore.confirm')}
        destructive
        errorText={problemText}
        onConfirm={async () => {
          if (!restoring) return;
          await restore.mutateAsync(restoring.name);
          setRestarted(true);
        }}
      />
    </section>
  );
}
