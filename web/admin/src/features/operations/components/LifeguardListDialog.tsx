import type { Schemas } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { DataTable, FormDialog, QueryState, type ColumnDef } from '@arenile/shared/ui/patterns';
import { EuroIcon, LogInIcon, LogOutIcon, PrinterIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { createPortal, flushSync } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { printIsolated } from '../../daily-sale/printing';
import { useLifeguardList } from '../api/m8';
import type { SlotChoice } from '../lib/status';

type LifeguardRow = Schemas['LifeguardRow'];
type LifeguardList = Schemas['LifeguardList'];

export interface LifeguardListDialogProps {
  establishmentId: string;
  date: string;
  slot: SlotChoice;
  mapId?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * List for the lifeguards (spec 30.10): per row, the spot, the surname,
 * the setup, arrival/departure and "da saldare", grouped by row. Never an
 * email or a phone. "Stampa" prints it with the browser (works offline).
 */
export function LifeguardListDialog({
  establishmentId,
  date,
  slot,
  mapId,
  open,
  onOpenChange,
}: LifeguardListDialogProps) {
  const { t } = useTranslation('operations');
  const list = useLifeguardList(establishmentId, date, slot, mapId, open);
  const [printing, setPrinting] = useState(false);
  const columns = useLifeguardColumns();

  const print = () => {
    flushSync(() => setPrinting(true));
    const target = document.querySelector('[data-lifeguard-print]');
    if (target) printIsolated(target);
  };

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifeguard.title')}
      description={t('lifeguard.description', {
        date: formatIsoDate(date),
        slot: t(`slot.${slot}`),
      })}
      wide
    >
      <div className="space-y-4">
        <QueryState
          isLoading={list.isLoading}
          error={list.error}
          onRetry={() => void list.refetch()}
          rows={6}
        />
        {list.data && (
          <DataTable
            caption={t('lifeguard.title')}
            columns={columns}
            data={list.data.rows}
            groupBy={(r) => r.row_label || '—'}
            renderGroup={(key) => t('lifeguard.row', { row: key })}
            getRowId={(r) => r.spot_id}
            pageSize={0}
          />
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
          <Button onClick={print} disabled={!list.data}>
            <PrinterIcon aria-hidden="true" />
            {t('lifeguard.print')}
          </Button>
        </DialogFooter>
      </div>
      {printing &&
        list.data &&
        createPortal(
          <div className="hidden print:block" data-lifeguard-print="">
            <LifeguardSheet list={list.data} />
          </div>,
          document.body,
        )}
    </FormDialog>
  );
}

function Flags({ row }: { row: LifeguardRow }) {
  const { t } = useTranslation('operations');
  return (
    <span className="flex flex-wrap items-center gap-2">
      {row.arriving && (
        <span className="flex items-center gap-1">
          <LogInIcon aria-hidden="true" className="size-4" />
          {t('indicator.arrival')}
        </span>
      )}
      {row.departing && (
        <span className="flex items-center gap-1">
          <LogOutIcon aria-hidden="true" className="size-4" />
          {t('indicator.departure')}
        </span>
      )}
    </span>
  );
}

function useLifeguardColumns(): ColumnDef<LifeguardRow>[] {
  const { t } = useTranslation('operations');
  return useMemo(
    () => [
      {
        accessorKey: 'spot_label',
        header: t('lifeguard.spot'),
        meta: { label: t('lifeguard.spot') },
      },
      {
        accessorKey: 'occupant_label',
        header: t('lifeguard.surname'),
        meta: { label: t('lifeguard.surname') },
        cell: ({ row }) => row.original.occupant_label || '—',
      },
      {
        accessorKey: 'setup_label',
        header: t('lifeguard.setup'),
        meta: { label: t('lifeguard.setup') },
        cell: ({ row }) => row.original.setup_label || '—',
      },
      {
        id: 'flags',
        header: t('lifeguard.arrivalDeparture'),
        meta: { label: t('lifeguard.arrivalDeparture') },
        cell: ({ row }) => <Flags row={row.original} />,
      },
      {
        accessorKey: 'balance_due',
        header: t('lifeguard.balance'),
        meta: { label: t('lifeguard.balance') },
        cell: ({ row }) =>
          row.original.balance_due ? (
            <span className="flex items-center gap-1 font-medium text-destructive">
              <EuroIcon aria-hidden="true" className="size-4" />
              {t('lifeguard.toCollect')}
            </span>
          ) : (
            ''
          ),
      },
    ],
    [t],
  );
}

/** The printed list: plain table by row, readable in black and white. */
function LifeguardSheet({ list }: { list: LifeguardList }) {
  const { t } = useTranslation('operations');
  const rows = new Map<string, LifeguardRow[]>();
  for (const r of list.rows) {
    const key = r.row_label || '—';
    rows.set(key, [...(rows.get(key) ?? []), r]);
  }
  return (
    <article className="space-y-3 text-sm print:text-black">
      <h1 className="text-lg font-semibold">
        {t('lifeguard.title')} · {formatIsoDate(list.date)} · {t(`slot.${list.slot}`)}
      </h1>
      <table className="w-full border-collapse text-left">
        <thead>
          <tr className="border-b">
            <th className="py-1 pr-2">{t('lifeguard.spot')}</th>
            <th className="py-1 pr-2">{t('lifeguard.surname')}</th>
            <th className="py-1 pr-2">{t('lifeguard.setup')}</th>
            <th className="py-1 pr-2">{t('lifeguard.arrivalDeparture')}</th>
            <th className="py-1">{t('lifeguard.balance')}</th>
          </tr>
        </thead>
        {[...rows].map(([key, items]) => (
          <tbody key={key}>
            <tr>
              <th colSpan={5} className="pt-2 text-left font-semibold">
                {t('lifeguard.row', { row: key })}
              </th>
            </tr>
            {items.map((r) => (
              <tr key={r.spot_id} className="border-b">
                <td className="py-0.5 pr-2">{r.spot_label}</td>
                <td className="py-0.5 pr-2">{r.occupant_label}</td>
                <td className="py-0.5 pr-2">{r.setup_label}</td>
                <td className="py-0.5 pr-2">
                  {[r.arriving && t('indicator.arrival'), r.departing && t('indicator.departure')]
                    .filter(Boolean)
                    .join(', ')}
                </td>
                <td className="py-0.5">{r.balance_due ? t('lifeguard.toCollect') : ''}</td>
              </tr>
            ))}
          </tbody>
        ))}
      </table>
    </article>
  );
}
