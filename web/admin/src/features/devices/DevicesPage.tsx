import { useMe, useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import {
  type ColumnDef,
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  Page,
  QueryState,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { Switch } from '@arenile/shared/ui/switch';
import { BanIcon, MonitorIcon, QrCodeIcon, SmartphoneIcon, WifiOffIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useCurrentEstablishment,
  useInstantFormat,
  useRequiredEstablishmentId,
} from '../../establishment';
import { useDevices, useRevokeDevice, type Device } from './api';
import { byLastActivity } from './lib';
import { PairDeviceDialog } from './PairDeviceDialog';
import { PushSettingsCard } from './PushSettingsCard';
import { usePairingAvailability } from './usePairingAvailability';

/** Whether the caller holds perm on the establishment (role or API key scope). */
function useHasPermission(establishmentId: string, perm: string): boolean {
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  if (!me.data || !establishment || establishment.id !== establishmentId) return false;
  if (me.data.principal_type === 'api_key') {
    return me.data.api_key?.scopes.some((s) => s === perm) ?? false;
  }
  return me.data.memberships.some(
    (m) =>
      (m.establishment_id
        ? m.establishment_id === establishmentId
        : m.organization_id === establishment.organization_id) &&
      m.permissions.some((p) => p === perm),
  );
}

/**
 * Configurazione → Dispositivi (spec 29.2): phones, tablets and desktop apps
 * in remote mode paired with this establishment. Every user sees and
 * revokes their own devices; owners (users.manage) see everyone's and can
 * pair a device for another member. "Abbina un dispositivo" shows a
 * single-use QR with a countdown. Desktop edition in local mode: pairing
 * needs the LAN mode (explained, button disabled). Owners and managers also
 * turn the push notifications of the mobile app on or off (ADR 0027).
 */
export function DevicesPage() {
  const { t } = useTranslation('devices');
  const establishmentId = useRequiredEstablishmentId();
  const me = useMe();
  // users.manage (owners): sees and revokes every device.
  const canManageUsers = useHasPermission(establishmentId, 'users.manage');
  // establishment.configure (owners, managers): push notification settings.
  const canConfigure = useHasPermission(establishmentId, 'establishment.configure');
  const { block } = usePairingAvailability();
  const fmt = useInstantFormat();
  const problemText = useProblemText();
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const [pairOpen, setPairOpen] = useState(false);
  const [revoking, setRevoking] = useState<Device | null>(null);
  const devices = useDevices(establishmentId, { includeRevoked });
  const revoke = useRevokeDevice(establishmentId);
  const selfId = me.data?.user?.id;
  const rows = [...(devices.data ?? [])].sort(byLastActivity);

  const columns: ColumnDef<Device>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('fields.device') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.device')} />,
      cell: ({ row }) => {
        const d = row.original;
        const Icon = d.kind === 'desktop' ? MonitorIcon : SmartphoneIcon;
        return (
          <span className="flex items-center gap-3">
            <Icon aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />
            <span className="flex min-w-0 flex-col">
              <span className="flex flex-wrap items-center gap-1.5 font-medium">
                {d.name}
                {d.current && <StatusBadge tone="info">{t('current')}</StatusBadge>}
              </span>
              <span className="truncate text-xs text-muted-foreground">
                {t(`platforms.${d.platform}`)}
                {d.app_version ? ` · ${d.app_version}` : ''}
              </span>
            </span>
          </span>
        );
      },
    },
    {
      id: 'user',
      accessorFn: (d) => d.user_name,
      meta: { label: t('fields.user') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.user')} />,
      cell: ({ row }) => (
        <span className="flex min-w-0 flex-col">
          <span>{row.original.user_name}</span>
          <span className="text-xs text-muted-foreground">{t(`roles.${row.original.role}`)}</span>
        </span>
      ),
    },
    {
      id: 'last_used',
      accessorFn: (d) => d.last_used_at ?? '',
      meta: { label: t('fields.lastUsed') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.lastUsed')} />
      ),
      cell: ({ row }) => (
        <span className="text-sm whitespace-nowrap">
          {row.original.last_used_at ? fmt.dateTime(row.original.last_used_at) : t('neverUsed')}
        </span>
      ),
    },
    {
      id: 'status',
      meta: { label: t('fields.status') },
      header: t('fields.status'),
      cell: ({ row }) => {
        const d = row.original;
        if (d.revoked_at) {
          return <StatusBadge tone="neutral">{t('status.revoked')}</StatusBadge>;
        }
        return (
          <span className="flex flex-wrap gap-1.5">
            <StatusBadge tone="success">{t('status.active')}</StatusBadge>
            {d.fiscal_bridge && <StatusBadge tone="info">{t('status.fiscalBridge')}</StatusBadge>}
          </span>
        );
      },
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const d = row.original;
        if (d.revoked_at) return null;
        return (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              aria-label={t('revoke.aria', { name: d.name })}
              onClick={() => setRevoking(d)}
            >
              <BanIcon aria-hidden="true" />
              <span className="hidden sm:inline">{t('revoke.action')}</span>
            </Button>
          </div>
        );
      },
    },
  ];

  const pairButton = (
    <Button onClick={() => setPairOpen(true)} disabled={block !== null}>
      <QrCodeIcon aria-hidden="true" />
      {t('pair.open')}
    </Button>
  );

  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={pairButton}
      notice={
        block === 'lan_mode_required' && (
          <Alert role="note">
            <WifiOffIcon aria-hidden="true" />
            <AlertTitle>{t('lan.title')}</AlertTitle>
            <AlertDescription>{t('lan.description')}</AlertDescription>
          </Alert>
        )
      }
    >
      {devices.error ? (
        <QueryState
          isLoading={false}
          error={devices.error}
          onRetry={() => void devices.refetch()}
        />
      ) : (
        <DataTable
          caption={t('title')}
          columns={columns}
          data={rows}
          loading={devices.isPending}
          getRowId={(d) => d.id}
          searchable
          toolbar={
            <div className="flex items-center gap-2">
              <Switch
                id="devices-revoked"
                checked={includeRevoked}
                onCheckedChange={setIncludeRevoked}
              />
              <Label htmlFor="devices-revoked" className="text-sm font-normal">
                {t('showRevoked')}
              </Label>
            </div>
          }
          empty={
            <EmptyState
              icon={SmartphoneIcon}
              title={t('empty.title')}
              description={canManageUsers ? t('empty.descriptionOwner') : t('empty.description')}
              bordered={false}
            />
          }
        />
      )}
      {canConfigure && <PushSettingsCard establishmentId={establishmentId} />}
      <PairDeviceDialog
        open={pairOpen}
        onOpenChange={setPairOpen}
        establishmentId={establishmentId}
        selfId={selfId}
        canManageUsers={canManageUsers}
        knownDevices={devices.data ?? []}
      />
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => {
          if (!open) setRevoking(null);
        }}
        title={t('revoke.title', { name: revoking?.name ?? '' })}
        description={revoking?.fiscal_bridge ? t('revoke.messageBridge') : t('revoke.message')}
        confirmLabel={t('revoke.action')}
        destructive
        errorText={problemText}
        onConfirm={async () => {
          if (!revoking) return;
          const name = revoking.name;
          await revoke.mutateAsync(revoking.id);
          toast.success(t('revoke.done', { name }));
        }}
      />
    </Page>
  );
}
