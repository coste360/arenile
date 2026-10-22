import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Avatar, AvatarFallback } from '@arenile/shared/ui/avatar';
import { Button } from '@arenile/shared/ui/button';
import { FieldDescription } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  DeleteButton,
  EmptyState,
  FormActions,
  FormDialog,
  QueryState,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  CopyIcon,
  KeyRoundIcon,
  LockIcon,
  ShieldCheckIcon,
  UserPlusIcon,
  UsersIcon,
  XIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useInviteMember,
  useRemoveMember,
  useTeam,
  useUpdateMember,
  type Role,
  type TeamMember,
  type TeamMemberCreated,
} from '../api/team';
import { useConfigContext } from '../api/useConfigContext';
import { useEditor } from '../api/useCrud';
import { ConfigPage } from '../components/ConfigPage';
import { useProblemText } from '@arenile/shared/api';
import { Field, SelectField } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { fieldAria } from '../lib/aria';
import { problemToFormState, type FormAlertState } from '../lib/formErrors';
import {
  ROLES,
  TEAM_SCOPES,
  inviteDefaults,
  inviteSchema,
  toTeamMemberCreate,
} from './team.schema';

function initials(name: string): string {
  return (
    name
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((p) => p[0]?.toUpperCase())
      .join('') || '?'
  );
}

/** Configurazione → Utenti: owner only (spec 16.1 "Utenti e ruoli"). */
export function TeamPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const me = useMe();
  const allowed = ctx.canManageUsers;
  const team = useTeam(ctx.establishmentId, allowed);
  const update = useUpdateMember(ctx.establishmentId);
  const remove = useRemoveMember(ctx.establishmentId);
  const editor = useEditor<never>();
  const problemText = useProblemText();
  const [created, setCreated] = useState<TeamMemberCreated | null>(null);

  if (ctx.isLoading) {
    return (
      <ConfigPage title={t('team.title')}>
        <QueryState isLoading error={null} />
      </ConfigPage>
    );
  }
  if (!allowed) {
    return (
      <ConfigPage title={t('team.title')}>
        <EmptyState icon={LockIcon} title={t('team.ownerOnly')} />
      </ConfigPage>
    );
  }

  const selfId = me.data?.user?.id;
  const patch = (m: TeamMember, body: Parameters<typeof update.mutate>[0]['patch']) => {
    update.mutate(
      { userId: m.user_id, patch: body },
      {
        onSuccess: () => toast.success(t('toast.saved')),
        onError: (err) => toast.error(problemText(err)),
      },
    );
  };
  const members = [...(team.data ?? [])].sort((a, b) => a.name.localeCompare(b.name));

  const columns: ColumnDef<TeamMember>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('team.fields.name') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('team.fields.name')} />
      ),
      cell: ({ row }) => {
        const m = row.original;
        return (
          <span className="flex items-center gap-3">
            <Avatar className="size-8">
              <AvatarFallback className="text-xs">{initials(m.name)}</AvatarFallback>
            </Avatar>
            <span className="flex min-w-0 flex-col">
              <span className="flex items-center gap-1.5 font-medium">
                {m.name}
                {m.totp_enabled && (
                  <ShieldCheckIcon
                    aria-label={t('team.totp')}
                    role="img"
                    className="size-3.5 text-success"
                  />
                )}
              </span>
              <span className="truncate text-xs text-muted-foreground">{m.email}</span>
            </span>
          </span>
        );
      },
    },
    {
      accessorKey: 'role',
      meta: { label: t('team.fields.role') },
      header: t('team.fields.role'),
      cell: ({ row }) => {
        const m = row.original;
        return (
          <Select
            value={m.role}
            disabled={m.user_id === selfId || update.isPending}
            onValueChange={(v) => patch(m, { role: v as Role, scope: m.scope })}
          >
            <SelectTrigger
              size="sm"
              className="w-44"
              aria-label={t('team.roleOf', { name: m.name })}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ROLES.map((r) => (
                <SelectItem key={r} value={r}>
                  {t(`roles.${r}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        );
      },
    },
    {
      accessorKey: 'scope',
      meta: { label: t('team.fields.scope') },
      header: t('team.fields.scope'),
      cell: ({ row }) => (
        <span className="text-muted-foreground">{t(`team.scopes.${row.original.scope}`)}</span>
      ),
    },
    {
      accessorKey: 'active',
      meta: { label: t('team.status') },
      header: t('team.status'),
      cell: ({ row }) =>
        row.original.active ? (
          <StatusBadge tone="success">{t('team.active')}</StatusBadge>
        ) : (
          <StatusBadge tone="warning">{t('team.inactive')}</StatusBadge>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const m = row.original;
        if (m.user_id === selfId) return null;
        return (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              disabled={update.isPending}
              onClick={() => patch(m, { active: !m.active })}
              aria-label={
                m.active
                  ? t('team.deactivateAria', { name: m.name })
                  : t('team.reactivateAria', { name: m.name })
              }
            >
              {m.active ? t('team.deactivate') : t('team.reactivate')}
            </Button>
            <DeleteButton
              label={t('team.removeAria', { name: m.name })}
              message={t('team.removeConfirm', { name: m.name })}
              onConfirm={() => remove.mutateAsync(m.user_id)}
            />
          </div>
        );
      },
    },
  ];

  const inviteButton = (
    <Button onClick={editor.create}>
      <UserPlusIcon aria-hidden="true" />
      {t('team.invite')}
    </Button>
  );

  return (
    <ConfigPage title={t('team.title')} description={t('team.description')} actions={inviteButton}>
      {created && <CreatedNotice created={created} onClose={() => setCreated(null)} />}
      {team.error ? (
        <QueryState isLoading={false} error={team.error} onRetry={() => void team.refetch()} />
      ) : (
        <DataTable
          caption={t('team.title')}
          columns={columns}
          data={members}
          loading={team.isPending}
          getRowId={(m) => m.user_id}
          searchable
          empty={<EmptyState icon={UsersIcon} title={t('team.empty')} bordered={false} />}
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={t('team.invite')}
        description={t('team.inviteHint')}
      >
        <InviteForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          onDone={(res) => {
            setCreated(res);
            editor.close();
          }}
          onCancel={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

/** Result of an invitation; the initial password is shown once with a copy button. */
function CreatedNotice({ created, onClose }: { created: TeamMemberCreated; onClose: () => void }) {
  const { t } = useTranslation('configuration');
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(t('embed.copied'));
    } catch {
      // Clipboard denied: the password stays selectable.
    }
  };
  return (
    <Alert role="status" className="border-success/40 bg-success/10">
      <KeyRoundIcon aria-hidden="true" />
      <AlertTitle className="pr-8">
        {created.user_created
          ? t('team.createdNew', { name: created.member.name })
          : t('team.createdExisting', { name: created.member.name })}
      </AlertTitle>
      {created.initial_password && (
        <AlertDescription className="mt-2 space-y-2">
          <p>{t('team.initialPassword')}</p>
          <div className="flex items-center gap-2">
            <code className="rounded-md border bg-background px-2 py-1 font-mono text-sm text-foreground select-all">
              {created.initial_password}
            </code>
            <Button
              size="icon-sm"
              variant="outline"
              aria-label={t('embed.copy')}
              onClick={() => void copy(created.initial_password ?? '')}
            >
              <CopyIcon aria-hidden="true" />
            </Button>
          </div>
        </AlertDescription>
      )}
      <Button
        size="icon-sm"
        variant="ghost"
        className="absolute top-2 right-2"
        aria-label={t('common.close', { ns: 'translation' })}
        onClick={onClose}
      >
        <XIcon aria-hidden="true" />
      </Button>
    </Alert>
  );
}

function InviteForm({
  establishmentId,
  onDone,
  onCancel,
}: {
  establishmentId: string;
  onDone: (res: TeamMemberCreated) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('configuration');
  const invite = useInviteMember(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const form = useForm({ resolver: zodResolver(inviteSchema), defaultValues: inviteDefaults });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      onDone(await invite.mutateAsync(toTeamMemberCreate(values)));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });
  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      <FormAlert state={alert} />
      <div className="grid gap-5 sm:grid-cols-2">
        <Field id="inv-email" label={t('team.fields.email')} error={errors.email}>
          <Input
            {...fieldAria('inv-email', errors.email)}
            type="email"
            autoComplete="off"
            {...register('email')}
          />
        </Field>
        <Field id="inv-name" label={t('team.fields.name')} error={errors.name}>
          <Input {...fieldAria('inv-name', errors.name)} autoComplete="off" {...register('name')} />
        </Field>
        <SelectField
          control={control}
          name="role"
          id="inv-role"
          label={t('team.fields.role')}
          options={ROLES.map((r) => ({ value: r, label: t(`roles.${r}`) }))}
        />
        <SelectField
          control={control}
          name="scope"
          id="inv-scope"
          label={t('team.fields.scope')}
          options={TEAM_SCOPES.map((s) => ({ value: s, label: t(`team.scopes.${s}`) }))}
        />
        <SelectField
          control={control}
          name="locale"
          id="inv-locale"
          label={t('team.fields.locale')}
          options={(['it', 'en', 'de', 'fr'] as const).map((l) => ({
            value: l,
            label: t(`common.languages.${l}`, { ns: 'translation' }),
          }))}
        />
        <Field
          id="inv-password"
          label={t('team.fields.password')}
          error={errors.password}
          hint={t('team.passwordHint')}
        >
          <Input
            {...fieldAria('inv-password', errors.password, true)}
            type="password"
            autoComplete="new-password"
            {...register('password')}
          />
        </Field>
      </div>
      <FieldDescription>{t('team.rolesHint')}</FieldDescription>
      <FormActions
        onCancel={onCancel}
        pending={invite.isPending}
        submitLabel={t('team.inviteSubmit')}
      />
    </form>
  );
}
