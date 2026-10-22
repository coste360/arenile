import { Button, Card } from '@arenile/shared/ui';
import { Link, useNavigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { maps } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import { ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { ConfirmDelete, FormDialog } from '../configuration/components/dialogs';
import type { FormAlertState } from '../configuration/lib/formErrors';
import { problemToFormState } from '../configuration/lib/formErrors';
import { localize } from '../configuration/lib/i18nText';
import { MapSettingsForm } from './dialogs';
import type { ApiMap } from './model/types';

/** Configurazione → Mappe: list, create, delete and open the editor. */
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
  const items = [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const name = (m: ApiMap) => localize(m.name, ctx.uiLocale, ctx.defaultLocale);

  return (
    <ConfigPage
      title={t('maps.title')}
      description={t('maps.description')}
      readOnly={readOnly}
      actions={
        !readOnly && (
          <Button
            onClick={() => {
              setAlert(null);
              editor.create();
            }}
          >
            {t('maps.add')}
          </Button>
        )
      }
    >
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('maps.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[36rem] text-sm">
            <caption className="sr-only">{t('maps.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('fields.name')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('maps.size')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('fields.sea_side')}
                </th>
                <th scope="col" className="px-4 py-2 text-right font-medium">
                  {t('maps.spots')}
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">{t('maps.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((m) => (
                <tr key={m.id} className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-medium">
                    <Link
                      to="/e/$establishmentId/maps/$mapId"
                      params={{ establishmentId: ctx.establishmentId, mapId: m.id }}
                      className="text-[var(--ar-primary)] underline-offset-2 hover:underline"
                    >
                      {name(m)}
                    </Link>
                  </th>
                  <td className="px-4 py-2 tabular-nums">
                    {m.width} × {m.height}
                  </td>
                  <td className="px-4 py-2">{t(`seaSides.${m.sea_side}`)}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{m.spot_count}</td>
                  <td className="px-4 py-2 text-right">
                    <div className="flex justify-end gap-1">
                      <Button asChild variant="ghost" size="sm">
                        <Link
                          to="/e/$establishmentId/maps/$mapId"
                          params={{ establishmentId: ctx.establishmentId, mapId: m.id }}
                          aria-label={t('maps.openAria', { name: name(m) })}
                        >
                          {readOnly ? t('maps.view') : t('maps.open')}
                        </Link>
                      </Button>
                      {!readOnly && (
                        <ConfirmDelete
                          label={t('maps.deleteAria', { name: name(m) })}
                          message={t('maps.deleteConfirm', { name: name(m), count: m.spot_count })}
                          onConfirm={() => remove.mutateAsync({ id: m.id, version: m.version })}
                        />
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <FormDialog open={editor.open} onOpenChange={editor.setOpen} wide title={t('maps.add')}>
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
