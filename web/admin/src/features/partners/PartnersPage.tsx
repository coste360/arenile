import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import {
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  OptionSelect,
  Page,
  QueryState,
  ResponsiveTabsList,
  SectionCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  BuildingIcon,
  FileSpreadsheetIcon,
  HandshakeIcon,
  PlusIcon,
  UsersRoundIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { PARTNER_KINDS, partners, type Partner } from './api';
import { CustomerCategoriesTab } from './CustomerCategoriesTab';
import { usePartnerPermissions } from './lib';
import { PartnerFormSheet } from './PartnerForm';

type Tab = 'partners' | 'categories';

/**
 * "Partner e convenzioni" (spec 30.2): partners (accommodation, bodies,
 * companies, associations) and customer categories, with links to the
 * partner account and to the agreement reports. The agreement conditions are
 * edited in Prezzi → Sconti (pricing engine, discount stage).
 */
export function PartnersPage() {
  const { t } = useTranslation('partners');
  const establishmentId = useRequiredEstablishmentId();
  const perms = usePartnerPermissions();
  const [tab, setTab] = useState<Tab>('partners');
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();

  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={
        <>
          {perms.canReport && (
            <Button variant="outline" asChild>
              <Link to="/e/$establishmentId/convenzioni/rendiconti" params={{ establishmentId }}>
                <FileSpreadsheetIcon aria-hidden="true" />
                {t('tabs.reports')}
              </Link>
            </Button>
          )}
          {perms.canConfigure && tab === 'partners' && (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon aria-hidden="true" />
              {t('add')}
            </Button>
          )}
        </>
      }
    >
      <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="gap-4">
        <ResponsiveTabsList
          label={t('title')}
          value={tab}
          onValueChange={(v) => setTab(v as Tab)}
          options={[
            {
              value: 'partners',
              label: t('tabs.partners'),
              icon: BuildingIcon,
            },
            {
              value: 'categories',
              label: t('tabs.categories'),
              icon: UsersRoundIcon,
            },
          ]}
        />
        <TabsContent value="partners" className="space-y-4">
          <PartnersTable establishmentId={establishmentId} />
          <p className="text-sm text-muted-foreground">
            {t('agreementsHint')}{' '}
            <Link
              to="/e/$establishmentId/pricing"
              params={{ establishmentId }}
              className="font-medium text-primary-on-surface underline-offset-4 hover:underline"
            >
              {t('agreementsLink')}
            </Link>
          </p>
        </TabsContent>
        <TabsContent value="categories">
          <CustomerCategoriesTab
            establishmentId={establishmentId}
            canConfigure={perms.canConfigure}
          />
        </TabsContent>
      </Tabs>
      <PartnerFormSheet
        establishmentId={establishmentId}
        open={creating}
        onOpenChange={setCreating}
        onSaved={(p) =>
          void navigate({
            to: '/e/$establishmentId/convenzioni/partner/$partnerId',
            params: { establishmentId, partnerId: p.id },
          })
        }
      />
    </Page>
  );
}

function PartnersTable({ establishmentId }: { establishmentId: string }) {
  const { t } = useTranslation('partners');
  const navigate = useNavigate();
  const [kind, setKind] = useState('all');
  const [showInactive, setShowInactive] = useState(false);
  const list = partners.useList(establishmentId);
  const rows = (list.data ?? []).filter(
    (p) => (showInactive || p.active) && (kind === 'all' || p.kind === kind),
  );
  const open = (p: Partner) =>
    void navigate({
      to: '/e/$establishmentId/convenzioni/partner/$partnerId',
      params: { establishmentId, partnerId: p.id },
    });
  const columns: ColumnDef<Partner>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row: { original: p } }) => (
        <span className="flex flex-wrap items-center gap-2">
          <Link
            to="/e/$establishmentId/convenzioni/partner/$partnerId"
            params={{ establishmentId, partnerId: p.id }}
            className="font-medium text-primary-on-surface underline-offset-4 hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {p.name}
          </Link>
          {!p.active && <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>}
        </span>
      ),
    },
    {
      id: 'kind',
      meta: { label: t('fields.kind') },
      header: t('fields.kind'),
      accessorFn: (p) =>
        p.accommodation_type
          ? `${t(`kinds.${p.kind}`)} · ${t(`accommodation.${p.accommodation_type}`)}`
          : t(`kinds.${p.kind}`),
    },
    {
      id: 'municipality',
      meta: { label: t('fields.municipality') },
      header: t('fields.municipality'),
      accessorFn: (p) => p.municipality || '—',
    },
    {
      id: 'agreements',
      meta: { label: t('columns.agreements'), align: 'right' },
      header: t('columns.agreements'),
      accessorFn: (p) => p.agreement_count ?? 0,
    },
    {
      id: 'balance',
      meta: { label: t('columns.balance'), align: 'right' },
      header: t('columns.balance'),
      cell: ({ row: { original: p } }) =>
        p.balance === undefined ? (
          '—'
        ) : (
          <span
            className={
              p.balance > 0
                ? 'font-medium text-destructive'
                : p.balance === 0
                  ? 'text-muted-foreground'
                  : undefined
            }
          >
            {formatCents(p.balance)}
          </span>
        ),
    },
  ];
  return (
    <SectionCard flush icon={HandshakeIcon} title={t('tabs.partners')}>
      <DataTable
        embedded
        caption={t('tabs.partners')}
        loading={list.isPending}
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        columns={columns}
        data={rows}
        getRowId={(p) => p.id}
        onRowClick={open}
        getRowLabel={(p) => p.name}
        searchable
        filters={
          <>
            <OptionSelect
              className="w-full sm:w-52"
              aria-label={t('fields.kind')}
              value={kind}
              onChange={setKind}
              options={[
                { value: 'all', label: t('filters.allKinds') },
                ...PARTNER_KINDS.map((k) => ({ value: k, label: t(`kinds.${k}`) })),
              ]}
            />
            <div className="flex items-center gap-2">
              <Switch
                id="partners-inactive"
                checked={showInactive}
                onCheckedChange={setShowInactive}
              />
              <Label htmlFor="partners-inactive" className="font-normal">
                {t('filters.inactive')}
              </Label>
            </div>
          </>
        }
        initialSorting={[{ id: 'name', desc: false }]}
        empty={
          <EmptyState
            icon={HandshakeIcon}
            bordered={false}
            title={t('empty')}
            description={t('emptyHint')}
          />
        }
      />
    </SectionCard>
  );
}
