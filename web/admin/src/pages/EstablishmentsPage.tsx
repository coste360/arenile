import { useEstablishmentPages, type Establishment } from '@arenile/shared/api';
import {
  DataTable,
  QueryState,
  SectionCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { ExternalLinkIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** Establishments accessible to the current user (M0 endpoint, cursor pagination). */
export function EstablishmentsPage() {
  const { t } = useTranslation();
  const query = useEstablishmentPages();
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];

  const columns: ColumnDef<Establishment>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('admin.establishments.name') },
      header: t('admin.establishments.name'),
      cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
    },
    {
      accessorKey: 'slug',
      meta: { label: t('admin.establishments.slug') },
      header: t('admin.establishments.slug'),
      cell: ({ row }) => {
        const e = row.original;
        return (
          <a
            href={`/b/${encodeURIComponent(e.slug)}`}
            className="inline-flex items-center gap-1 text-primary-on-surface underline-offset-4 hover:underline"
          >
            /b/{e.slug}
            {/* Visible text stays part of the accessible name (WCAG 2.5.3). */}
            <span className="sr-only">
              {' '}
              {t('admin.establishments.openPublicPage', { name: e.name })}
            </span>
            <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
          </a>
        );
      },
    },
    {
      id: 'locale',
      meta: { label: t('admin.establishments.locale') },
      header: t('admin.establishments.locale'),
      cell: ({ row }) =>
        t(`common.languages.${row.original.default_locale}`, {
          defaultValue: row.original.default_locale,
        }),
    },
    {
      id: 'booking',
      meta: { label: t('admin.establishments.booking') },
      header: t('admin.establishments.booking'),
      cell: ({ row }) => (
        <StatusBadge tone={row.original.booking_enabled ? 'success' : 'neutral'}>
          {row.original.booking_enabled
            ? t('admin.establishments.bookingOn')
            : t('admin.establishments.bookingOff')}
        </StatusBadge>
      ),
    },
  ];

  return (
    <section aria-labelledby="establishments-title">
      <SectionCard titleId="establishments-title" title={t('admin.establishments.title')} flush>
        <DataTable
          embedded
          caption={t('admin.establishments.title')}
          columns={columns}
          data={items}
          loading={query.isPending}
          error={
            query.isError && items.length === 0 ? (
              <QueryState
                isLoading={false}
                error={query.error}
                onRetry={() => void query.refetch()}
              />
            ) : undefined
          }
          getRowId={(e) => e.id}
          pageSize={0}
          columnToggle={false}
          empty={
            <p className="py-10 text-center text-sm text-muted-foreground">
              {t('admin.establishments.empty')}
            </p>
          }
          loadMore={{
            hasMore: !!query.hasNextPage,
            loading: query.isFetchingNextPage,
            label: t('admin.establishments.loadMore'),
            onLoadMore: () => void query.fetchNextPage(),
          }}
        />
        {query.isFetchNextPageError && (
          <div className="px-6 pb-6">
            <QueryState
              isLoading={false}
              error={query.error}
              onRetry={() => void query.fetchNextPage()}
            />
          </div>
        )}
      </SectionCard>
    </section>
  );
}
