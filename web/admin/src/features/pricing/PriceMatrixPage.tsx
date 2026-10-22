import { Alert, AlertDescription, Badge, Button, Label } from '@arenile/shared/ui';
import {
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { CalendarXIcon, DownloadIcon, LockIcon, Table2Icon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { formatCents } from '@arenile/shared/lib/money';
import { usePriceMatrix, type Channel, type Formula, type PriceMatrix, type Slots } from './api';
import { OptionSelect, PricingLayout, QueryState, SeasonSelect } from './kit';
import { downloadCsv, matrixToCsv } from './matrixCsv';
import { CHANNELS } from './rules.schema';
import { localize } from '../configuration/lib/i18nText';
import { useSelectedSeason } from './season';
import { usePricingData } from './usePricingData';

type Cell = PriceMatrix['cells'][number];
type MatrixRow = { id: string; pack: string; category: string; cells: Record<string, Cell> };

/**
 * Matrice di listino (spec 8.7): price categories × rate periods for every
 * pack, computed by the engine for a representative day.
 */
export function PriceMatrixPage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const [formula, setFormula] = useState<Formula>('day');
  const [channel, setChannel] = useState<Channel>('desk');
  const [slots, setSlots] = useState<Slots>('AM');
  const allowed = ctx.canConfigure;
  const matrix = usePriceMatrix(
    ctx.establishmentId,
    sel.season && allowed
      ? {
          season_id: sel.season.id,
          formula,
          channel,
          slots: formula === 'half_day' ? slots : 'FULL',
        }
      : null,
  );
  const packName = (id: string) => {
    const p = data.packs.find((x) => x.id === id);
    return p ? data.name(p.name) || p.code : id.slice(0, 8);
  };
  const exportCsv = () => {
    const m = matrix.data;
    if (!m || !sel.season) return;
    downloadCsv(
      `listino-${sel.season.year}-${m.formula}-${m.channel}${m.slots === 'FULL' ? '' : `-${m.slots}`}.csv`,
      matrixToCsv(m, {
        packName,
        locale: ctx.uiLocale,
        headers: { pack: t('fields.pack'), category: t('fields.price_category') },
        noPrice: t('matrix.noPrice'),
      }),
    );
  };
  return (
    <PricingLayout
      title={t('matrix.title')}
      description={t('matrix.description')}
      establishmentId={ctx.establishmentId}
      seasonId={sel.season?.id}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      {!ctx.isLoading && !allowed && (
        <Alert role="note">
          <LockIcon aria-hidden="true" />
          <AlertDescription>{t('configureOnly')}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap items-end gap-4">
        <LabeledSelect
          label={t('fields.formula')}
          value={formula}
          onChange={(v) => setFormula(v as Formula)}
          options={(['day', 'half_day'] as const).map((f) => ({
            value: f,
            label: t(`formulas.${f}`),
          }))}
        />
        {formula === 'half_day' && (
          <LabeledSelect
            label={t('fields.slots')}
            value={slots}
            onChange={(v) => setSlots(v as Slots)}
            options={(['AM', 'PM'] as const).map((s) => ({ value: s, label: t(`slots.${s}`) }))}
          />
        )}
        <LabeledSelect
          label={t('fields.channel')}
          value={channel}
          onChange={(v) => setChannel(v as Channel)}
          options={CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
        />
        <Button
          variant="outline"
          className="ml-auto"
          disabled={!matrix.data || matrix.data.pack_ids.length === 0}
          onClick={exportCsv}
        >
          <DownloadIcon aria-hidden="true" />
          {t('matrix.exportCsv')}
        </Button>
      </div>
      <QueryState
        isLoading={sel.isLoading || (allowed && !!sel.season && matrix.isPending)}
        error={sel.error ?? matrix.error}
        onRetry={() => void matrix.refetch()}
      />
      {!sel.isLoading && sel.seasons.length === 0 && (
        <EmptyState icon={CalendarXIcon} title={t('noSeason')} />
      )}
      {matrix.data && (
        <MatrixTable
          matrix={matrix.data}
          packName={packName}
          periodName={(code) => {
            const p = sel.periods.find((x) => x.code === code);
            return (p && localize(p.name, ctx.uiLocale, 'it')) || code;
          }}
        />
      )}
    </PricingLayout>
  );
}

function LabeledSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  const id = useId();
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <OptionSelect id={id} value={value} onChange={onChange} options={options} className="w-40" />
    </div>
  );
}

function MatrixTable({
  matrix,
  packName,
  periodName,
}: {
  matrix: PriceMatrix;
  packName: (id: string) => string;
  periodName: (code: string) => string;
}) {
  const { t } = useTranslation('pricing');
  if (matrix.pack_ids.length === 0 || matrix.categories.length === 0) {
    return <EmptyState icon={Table2Icon} title={t('matrix.empty')} />;
  }
  const rows: MatrixRow[] = matrix.pack_ids.flatMap((pack) =>
    matrix.categories.map((category) => ({
      id: `${pack}\u0000${category}`,
      pack: packName(pack),
      category,
      cells: Object.fromEntries(
        matrix.cells
          .filter((c) => c.pack_id === pack && c.price_category === category)
          .map((c) => [c.rate_period, c]),
      ),
    })),
  );
  const columns: ColumnDef<MatrixRow>[] = [
    {
      accessorKey: 'pack',
      meta: { label: t('fields.pack') },
      enableHiding: false,
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.pack')} />,
      cell: ({ row }) => <span className="font-medium">{row.original.pack}</span>,
    },
    {
      accessorKey: 'category',
      meta: { label: t('fields.price_category') },
      enableHiding: false,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.price_category')} />
      ),
      cell: ({ row }) => (
        <Badge variant="secondary" className="font-mono">
          {row.original.category}
        </Badge>
      ),
    },
    ...matrix.rate_periods.map((period): ColumnDef<MatrixRow> => ({
      id: `period:${period}`,
      meta: { label: periodName(period) },
      accessorFn: (r) => (r.cells[period]?.priced ? r.cells[period].price : -1),
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={periodName(period)} className="justify-end" />
      ),
      cell: ({ row }) => {
        const c = row.original.cells[period];
        return (
          <div className="text-right tabular-nums" title={c?.date}>
            {!c ? (
              <span className="text-muted-foreground">—</span>
            ) : c.priced ? (
              formatCents(c.price)
            ) : (
              <Badge variant="destructive">{t('matrix.noPrice')}</Badge>
            )}
          </div>
        );
      },
    })),
  ];
  return (
    <div className="space-y-2">
      <DataTable
        caption={t('matrix.title')}
        columns={columns}
        data={rows}
        getRowId={(r) => r.id}
        pageSize={0}
        searchable
        className="[&_[data-slot=table-container]]:max-h-[70vh] [&_thead]:sticky [&_thead]:top-0 [&_thead]:z-10 [&_thead]:bg-muted"
      />
      <p className="text-xs text-muted-foreground">{t('matrix.note')}</p>
    </div>
  );
}
