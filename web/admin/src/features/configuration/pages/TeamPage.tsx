import { useMe } from '@arenile/shared/api';
import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../../../lib/problem';
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
import { Badge, ConfigPage, EmptyState, QueryState } from '../components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Field, Select } from '../components/fields';
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
  const [created, setCreated] = useState<TeamMemberCreated | null>(null);
  const [rowError, setRowError] = useState<unknown>(null);

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
        <p
          role="alert"
          className="rounded-md border border-slate-200 bg-slate-100 px-3 py-2 text-sm"
        >
          {t('team.ownerOnly')}
        </p>
      </ConfigPage>
    );
  }

  const selfId = me.data?.user?.id;
  const patch = (m: TeamMember, body: Parameters<typeof update.mutate>[0]['patch']) => {
    setRowError(null);
    update.mutate({ userId: m.user_id, patch: body }, { onError: setRowError });
  };
  const members = [...(team.data ?? [])].sort((a, b) => a.name.localeCompare(b.name));

  return (
    <ConfigPage
      title={t('team.title')}
      description={t('team.description')}
      actions={<Button onClick={editor.create}>{t('team.invite')}</Button>}
    >
      {created && (
        <div
          role="status"
          className="space-y-1 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-900"
        >
          <p>
            {created.user_created
              ? t('team.createdNew', { name: created.member.name })
              : t('team.createdExisting', { name: created.member.name })}
          </p>
          {created.initial_password && (
            <p>
              {t('team.initialPassword')}{' '}
              <code className="select-all rounded bg-white px-1.5 py-0.5 font-mono">
                {created.initial_password}
              </code>
            </p>
          )}
          <Button size="sm" variant="ghost" onClick={() => setCreated(null)}>
            {t('common.close', { ns: 'translation' })}
          </Button>
        </div>
      )}
      {rowError ? (
        <p role="alert" className="text-sm text-red-700">
          {rowError instanceof Error && 'detail' in rowError && rowError.detail
            ? String(rowError.detail)
            : t(problemMessageKey(rowError), { ns: 'translation' })}
        </p>
      ) : null}
      <QueryState
        isLoading={team.isPending}
        error={team.error}
        onRetry={() => void team.refetch()}
      />
      {team.data && members.length === 0 && <EmptyState>{t('team.empty')}</EmptyState>}
      {members.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[44rem] text-sm">
            <caption className="sr-only">{t('team.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('team.fields.name')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('team.fields.role')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('team.fields.scope')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('team.status')}
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">{t('actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {members.map((m) => {
                const self = m.user_id === selfId;
                return (
                  <tr key={m.user_id} className="border-t border-slate-100">
                    <th scope="row" className="px-4 py-2 text-left font-normal">
                      <span className="block font-medium">{m.name}</span>
                      <span className="text-xs text-slate-600">{m.email}</span>
                      {m.totp_enabled && (
                        <span className="ml-2">
                          <Badge tone="blue">{t('team.totp')}</Badge>
                        </span>
                      )}
                    </th>
                    <td className="px-4 py-2">
                      <label htmlFor={`role-${m.user_id}`} className="sr-only">
                        {t('team.roleOf', { name: m.name })}
                      </label>
                      <Select
                        id={`role-${m.user_id}`}
                        className="h-9 w-40"
                        value={m.role}
                        disabled={self || update.isPending}
                        onChange={(e) => patch(m, { role: e.target.value as Role, scope: m.scope })}
                      >
                        {ROLES.map((r) => (
                          <option key={r} value={r}>
                            {t(`roles.${r}`)}
                          </option>
                        ))}
                      </Select>
                    </td>
                    <td className="px-4 py-2">{t(`team.scopes.${m.scope}`)}</td>
                    <td className="px-4 py-2">
                      {m.active ? (
                        <Badge tone="green">{t('team.active')}</Badge>
                      ) : (
                        <Badge tone="amber">{t('team.inactive')}</Badge>
                      )}
                    </td>
                    <td className="px-4 py-2 text-right">
                      {!self && (
                        <div className="flex justify-end gap-1">
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
                          <ConfirmDelete
                            label={t('team.removeAria', { name: m.name })}
                            message={t('team.removeConfirm', { name: m.name })}
                            onConfirm={() => remove.mutateAsync(m.user_id)}
                          />
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
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
    <form noValidate onSubmit={submit} className="space-y-4">
      <FormAlert state={alert} />
      <div className="grid gap-4 sm:grid-cols-2">
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
        <Field id="inv-role" label={t('team.fields.role')} error={errors.role}>
          <Select {...fieldAria('inv-role', errors.role)} {...register('role')}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {t(`roles.${r}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="inv-scope" label={t('team.fields.scope')} error={errors.scope}>
          <Select {...fieldAria('inv-scope', errors.scope)} {...register('scope')}>
            {TEAM_SCOPES.map((s) => (
              <option key={s} value={s}>
                {t(`team.scopes.${s}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="inv-locale" label={t('team.fields.locale')} error={errors.locale}>
          <Select {...fieldAria('inv-locale', errors.locale)} {...register('locale')}>
            {(['it', 'en', 'de', 'fr'] as const).map((l) => (
              <option key={l} value={l}>
                {t(`common.languages.${l}`, { ns: 'translation' })}
              </option>
            ))}
          </Select>
        </Field>
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
      <p className="text-xs text-slate-600">{t('team.rolesHint')}</p>
      <FormActions
        onCancel={onCancel}
        pending={invite.isPending}
        submitLabel={t('team.inviteSubmit')}
      />
    </form>
  );
}
