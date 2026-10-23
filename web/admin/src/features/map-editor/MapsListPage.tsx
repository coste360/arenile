import { Badge, Button } from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  DeleteButton,
  EmptyState,
  FormDialog,
  QueryState,
} from '@arenile/shared/ui/patterns';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowUpIcon,
  EyeIcon,
  MapIcon,
  PencilRulerIcon,
  PlusIcon,
  type LucideIcon,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { maps } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import type { FormAlertState } from '../configuration/lib/formErrors';
import { problemToFormState } from '../configuration/lib/formErrors';
import { localize } from '../configuration/lib/i18nText';
import { MapSettingsForm } from './dialogs';
import type { ApiMap, SeaSide } from './model/types';
import { ConfigPage } from '../configuration/components/ConfigPage';

const SEA_ICONS: Record<SeaSide, LucideIcon> = {
  top: ArrowUpIcon,
  bottom: ArrowDownIcon,
  left: ArrowLeftIcon,
  right: ArrowRightIcon,
};

export function MapsListPage() {
  const { t } = useTranslation('mapEditor');
  const ctx = useConfigContext();
  const navigate = useNavigate();
  const list = maps.useList(ctx.establishmentId);
  const create = maps.useCreate(ctx.establishmentId);
  const remove = maps.useRemove(ctx.establishmentId);
  const editor = useEditor<ApiMap>();
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const readOnly = !ctx.isLoading && !ctx.canConfigure;
  const items = useMemo(
    () => [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order),
    [list.data],
  );
  const { uiLocale, defaultLocale, establishmentId } = ctx;

  const columns = useMemo<ColumnDef<ApiMap>[]>(() => {
    const name = (m: ApiMap) => localize(m.name, uiLocale, defaultLocale);
    const removeMap = remove.mutateAsync;
    return [
      {
        id: 'name',
        accessorFn: name,
        enableHiding: false,
        meta: { label: t('fields.name') },
        header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
        cell: ({ row }) => (
          <Link
            to="/e/$establishmentId/maps/$mapId"
            params={{ establishmentId, mapId: row.original.id }}
            className="flex items-center gap-3 font-medium hover:underline"
          >
            <span
              aria-hidden="true"
              className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary-on-surface"
            >
              <MapIcon className="size-4" />
            </span>
            {name(row.original)}
          </Link>
        ),
      },
      {
        id: 'size',
        accessorFn: (m) => m.width * m.height,
        meta: { label: t('maps.size') },
        header: ({ column }) => <DataTableColumnHeader column={column} title={t('maps.size')} />,
        cell: ({ row }) => (
          <span className="text-muted-foreground tabular-nums">
            {row.original.width} × {row.original.height}
          </span>
        ),
      },
      {
        id: 'sea_side',
        accessorFn: (m) => t(`seaSides.${m.sea_side}`),
        meta: { label: t('fields.sea_side') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('fields.sea_side')} />
        ),
        cell: ({ row }) => {
          const Icon = SEA_ICONS[row.original.sea_side];
          return (
            <Badge variant="outline">
              <Icon aria-hidden="true" />
              {t(`seaSides.${row.original.sea_side}`)}
            </Badge>
          );
        },
      },
      {
        id: 'spots',
        accessorKey: 'spot_count',
        meta: { label: t('maps.spots'), align: 'right' },
        header: ({ column }) => <DataTableColumnHeader column={column} title={t('maps.spots')} />,
      },
      {
        id: 'actions',
        enableHiding: false,
        enableSorting: false,
        header: () => <span className="sr-only">{t('maps.actions')}</span>,
        cell: ({ row }) => {
          const m = row.original;
          return (
            <div className="flex justify-end gap-1">
              <Button asChild variant="outline" size="sm">
                <Link
                  to="/e/$establishmentId/maps/$mapId"
                  params={{ establishmentId, mapId: m.id }}
                  aria-label={t('maps.openAria', { name: name(m) })}
                >
                  {readOnly ? (
                    <EyeIcon aria-hidden="true" />
                  ) : (
                    <PencilRulerIcon aria-hidden="true" />
                  )}
                  {readOnly ? t('maps.view') : t('maps.open')}
                </Link>
              </Button>
              {!readOnly && (
                <DeleteButton
                  label={t('maps.deleteAria', { name: name(m) })}
                  message={t('maps.deleteConfirm', { name: name(m), count: m.spot_count })}
                  onConfirm={() => removeMap({ id: m.id, version: m.version })}
                />
              )}
            </div>
          );
        },
      },
    ];
  }, [t, uiLocale, defaultLocale, establishmentId, readOnly, remove.mutateAsync]);

  const openCreate = () => {
    setAlert(null);
    editor.create();
  };

  return (
    <ConfigPage
      title={t('maps.title')}
      description={t('maps.description')}
      readOnly={readOnly}
      actions={
        !readOnly && (
          <Button onClick={openCreate}>
            <PlusIcon aria-hidden="true" />
            {t('maps.add')}
          </Button>
        )
      }
    >
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('maps.title')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(m) => m.id}
          searchable
          searchPlaceholder={t('maps.searchPlaceholder')}
          columnToggle={false}
          empty={
            <EmptyState
              icon={MapIcon}
              bordered={false}
              title={t('maps.emptyTitle')}
              description={t('maps.empty')}
              action={
                !readOnly && (
                  <Button variant="outline" onClick={openCreate}>
                    <PlusIcon aria-hidden="true" />
                    {t('maps.add')}
                  </Button>
                )
              }
            />
          }
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={t('maps.add')}
        description={t('settings.description')}
      >
        <MapSettingsForm
          key={editor.key}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          pending={create.isPending}
          alert={alert}
          submitLabel={t('maps.create')}
          onCancel={editor.close}
          onSubmit={async (v) => {
            setAlert(null);
            try {
              const created = await create.mutateAsync({ ...v, sort_order: items.length });
              editor.close();
              await navigate({
                to: '/e/$establishmentId/maps/$mapId',
                params: { establishmentId: ctx.establishmentId, mapId: created.id },
              });
            } catch (err) {
              setAlert(problemToFormState(err, () => undefined, { formKeys: [] }));
            }
          }}
        />
      </FormDialog>
    </ConfigPage>
  );
}
