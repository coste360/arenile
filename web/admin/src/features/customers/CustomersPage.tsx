import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import {
  DataTable,
  EmptyState,
  Page,
  QueryState,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { useNavigate } from '@tanstack/react-router';
import { MailIcon, PlusIcon, TagIcon, UsersRoundIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { useOperationalPermissions } from '../bookings/model';
import { useCustomers, type Customer } from './api';
import { CustomerFormDialog } from './CustomerDialogs';
import { useDebounced } from './lib';

/** Clienti (spec 15.1, 23): search, tags, consent, new customer. */
export function CustomersPage() {
  const { t } = useTranslation('customers');
  const establishmentId = useRequiredEstablishmentId();
  const navigate = useNavigate();
  const perms = useOperationalPermissions();
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState('');
  const [anonymized, setAnonymized] = useState(false);
  const [creating, setCreating] = useState(false);
  const q = useDebounced(search.trim(), 300);
  const tagFilter = useDebounced(tag.trim().toLowerCase(), 300);
  const list = useCustomers(establishmentId, { q, tag: tagFilter, include_anonymized: anonymized });
  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  const open = (c: Customer) =>
    void navigate({
      to: '/e/$establishmentId/clienti/$customerId',
      params: { establishmentId, customerId: c.id },
    });

  const columns: ColumnDef<Customer>[] = [
    {
      accessorKey: 'display_name',
      header: t('columns.name'),
      meta: { label: t('columns.name') },
      cell: ({ row }) => (
        <span className="flex items-center gap-2 font-medium">
          {row.original.display_name}
          {row.original.anonymized_at && (
            <StatusBadge tone="neutral">{t('anonymized')}</StatusBadge>
          )}
        </span>
      ),
    },
    {
      accessorKey: 'phone',
      header: t('columns.phone'),
      meta: { label: t('columns.phone') },
      cell: ({ row }) => <span className="tabular-nums">{row.original.phone ?? '—'}</span>,
    },
    {
      accessorKey: 'email',
      header: t('columns.email'),
      meta: { label: t('columns.email') },
      cell: ({ row }) => row.original.email ?? '—',
    },
    {
      id: 'tags',
      header: t('columns.tags'),
      meta: { label: t('columns.tags') },
      cell: ({ row }) => (
        <span className="flex flex-wrap gap-1">
          {row.original.tags.map((tg) => (
            <Badge key={tg} variant="outline" asChild>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setTag(tg);
                }}
                aria-label={t('filters.byTag', { tag: tg })}
              >
                {tg}
              </button>
            </Badge>
          ))}
        </span>
      ),
    },
    {
      id: 'consent',
      header: t('columns.consent'),
      meta: { label: t('columns.consent') },
      cell: ({ row }) =>
        row.original.marketing_consent ? (
          <StatusBadge tone="success">
            <MailIcon aria-hidden="true" className="size-3" />
            {t('consent.yes')}
          </StatusBadge>
        ) : (
          <span className="text-muted-foreground">{t('consent.no')}</span>
        ),
    },
  ];

  const filtered = !!q || !!tagFilter || anonymized;

  return (
    <Page
      title={t('list.title')}
      description={t('list.description')}
      actions={
        perms.canWrite && (
          <Button onClick={() => setCreating(true)}>
            <PlusIcon aria-hidden="true" />
            {t('list.new')}
          </Button>
        )
      }
    >
      <DataTable
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        caption={t('list.title')}
        columns={columns}
        data={rows}
        loading={list.isPending}
        pageSize={0}
        getRowId={(r) => r.id}
        onRowClick={open}
        getRowLabel={(r) => r.display_name}
        searchable
        searchPlaceholder={t('filters.searchPlaceholder')}
        searchValue={search}
        onSearchChange={setSearch}
        filters={
          <>
            <Label htmlFor="customers-tag" className="sr-only">
              {t('filters.tag')}
            </Label>
            <InputGroup className="w-full sm:w-48">
              <InputGroupAddon>
                <TagIcon aria-hidden="true" />
              </InputGroupAddon>
              <InputGroupInput
                id="customers-tag"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
                placeholder={t('filters.tag')}
              />
            </InputGroup>
            <div className="flex items-center gap-2">
              <Switch id="customers-anon" checked={anonymized} onCheckedChange={setAnonymized} />
              <Label htmlFor="customers-anon">{t('filters.anonymized')}</Label>
            </div>
          </>
        }
        loadMore={{
          hasMore: !!list.hasNextPage,
          loading: list.isFetchingNextPage,
          onLoadMore: () => void list.fetchNextPage(),
          label: t('list.loadMore'),
        }}
        empty={
          <EmptyState
            icon={UsersRoundIcon}
            bordered={false}
            title={filtered ? t('list.emptyFiltered') : t('list.empty')}
          />
        }
      />
      <CustomerFormDialog
        establishmentId={establishmentId}
        open={creating}
        onOpenChange={setCreating}
        onSaved={open}
      />
    </Page>
  );
}
