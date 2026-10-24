import { useMe } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { cn } from '@arenile/shared/lib/utils';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Kbd } from '@arenile/shared/ui/kbd';
import { Label } from '@arenile/shared/ui/label';
import {
  DatePicker,
  ExtraList,
  ExtraRow,
  MoneyInput,
  OptionSelect,
  QuantityStepper,
  SegmentedControl,
  useExtraUnitText,
  useQuantityMaxText,
} from '@arenile/shared/ui/patterns';
import { Separator } from '@arenile/shared/ui/separator';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { Link } from '@tanstack/react-router';
import {
  BanknoteIcon,
  CircleAlertIcon,
  CreditCardIcon,
  MapPinIcon,
  PrinterIcon,
  RotateCcwIcon,
  TicketCheckIcon,
  WalletIcon,
} from 'lucide-react';
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment } from '../../establishment';
import { useEstablishmentSpots } from '../bookings/spots';
import { useCurrentSession } from '../cash/api';
import { extras as extrasResource, packs as packsResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { useDebounced } from '../customers/lib';
import { useOperationsDay } from '../operations/api/operations';
import { grantedPermissions } from '../operations/lib/permissions';
import { isBookable, slotStatus } from '../operations/lib/status';
import {
  useCreateDailySale,
  useDailySaleQuote,
  useDailySaleSettings,
  useFetchTicketPrint,
  type DailySaleResult,
  type TicketPaymentMethod,
} from './api';
import {
  SALE_SLOTS,
  StableKey,
  changeDue,
  packsForSale,
  saleDefaults,
  shortcutFor,
  shortcutKey,
  toCreateRequest,
  toQuoteRequest,
  vatRateLabel,
  type SaleShortcut,
  type SaleSlots,
  type SaleValues,
  type SaleWhere,
} from './lib';
import { useTicketPrinter } from './printing';
import { TicketPrintDialog } from './TicketPrintDialog';
import { useSaleProblemText } from './useSaleProblemText';

export interface DailySaleFormProps {
  establishmentId: string;
  /** Day of the sale (YYYY-MM-DD, establishment timezone). */
  date: string;
  /** Spot-first: the free spot tapped on the map. */
  spotId?: string;
  /** Zone-first: preselected zone (e.g. "Prima fila"). */
  zoneId?: string;
  /** Slot shown on the map, as the default formula. */
  slots?: SaleSlots;
  /** `sheet`: one column with the payment bar at the bottom; `page`: two columns. */
  layout?: 'page' | 'sheet';
  /** Shows a "Chiudi" button after the sale (sheet). */
  onClose?: () => void;
  onSold?: (result: DailySaleResult) => void;
}

/**
 * The quick walk-in sale (spec 14.3): where (a precise spot or only a zone),
 * slot, pack, extras, optional name, live price from the engine, then one
 * tap on "Contanti" or "Carta" creates the confirmed booking, the manual
 * payment and the numbered ticket, and prints it. Desktop shortcuts: 1/2/3
 * slot, P/Z spot or zone, C cash, K card (outside text fields).
 */
export function DailySaleForm({
  establishmentId,
  date,
  spotId,
  zoneId,
  slots,
  layout = 'page',
  onClose,
  onSold,
}: DailySaleFormProps) {
  const { t, i18n } = useTranslation('dailySale');
  const locale = i18n.language || 'it';
  const name = useCallback(
    (text: Record<string, string> | undefined) => localize(text, locale, 'it'),
    [locale],
  );
  const ids = useId();
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  const canCash = grantedPermissions(me.data, establishment).has('cash.operate');
  const inventory = useEstablishmentSpots(establishmentId, locale);
  const packList = packsResource.useList(establishmentId);
  const extraList = extrasResource.useList(establishmentId);
  const settings = useDailySaleSettings(establishmentId);
  const session = useCurrentSession(establishmentId, undefined, canCash);
  const create = useCreateDailySale(establishmentId);
  const fetchPrint = useFetchTicketPrint(establishmentId);
  const printer = useTicketPrinter();
  const problemText = useSaleProblemText();
  const keyRef = useRef(new StableKey());

  const [values, setValues] = useState<SaleValues>(() =>
    saleDefaults(date, { spotId, zoneId, slots }),
  );
  // Packs for the chosen spot or zone (never the entry-ticket packs).
  const targetZoneId =
    values.where === 'zone'
      ? values.zoneId
      : (inventory.spotById.get(values.spotId)?.zone_id ?? '');
  const quantityZoneIds = useMemo(
    () => new Set(inventory.zones.filter((z) => z.assignment_mode === 'quantity').map((z) => z.id)),
    [inventory.zones],
  );
  const packs = useMemo(
    () =>
      packsForSale(packList.data ?? [], targetZoneId, quantityZoneIds).sort(
        (a, b) => a.sort_order - b.sort_order,
      ),
    [packList.data, targetZoneId, quantityZoneIds],
  );
  const [failure, setFailure] = useState<string | null>(null);
  const [sold, setSold] = useState<DailySaleResult | null>(null);
  const [printOpen, setPrintOpen] = useState(false);
  const patch = useCallback((p: Partial<SaleValues>) => {
    setFailure(null);
    setValues((v) => ({ ...v, ...p }));
  }, []);

  // First pack by default, once the catalogue is loaded.
  const packId = (packs.some((p) => p.id === values.packId) ? values.packId : packs[0]?.id) ?? '';
  const effective: SaleValues = { ...values, packId };

  const extras = (extraList.data ?? [])
    .filter((x) => x.active && (!x.allowed_pack_ids?.length || x.allowed_pack_ids.includes(packId)))
    .sort((a, b) => a.sort_order - b.sort_order);
  // Extras hidden by the chosen pack are never sent.
  const visibleExtraIds = new Set(extras.map((x) => x.id));
  effective.extras = Object.fromEntries(
    Object.entries(values.extras).filter(([id]) => visibleExtraIds.has(id)),
  );

  const zoneFirst = settings.data?.zone_first_enabled ?? true;
  const day = useOperationsDay(establishmentId, values.date, values.slots);
  const freeSpots = useMemo(() => {
    const out = (day.data?.spots ?? []).filter((s) => isBookable(slotStatus(s, values.slots)));
    return out.sort((a, b) => a.label.localeCompare(b.label, undefined, { numeric: true }));
  }, [day.data, values.slots]);
  const freeByZone = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of freeSpots) m.set(s.zone_id, (m.get(s.zone_id) ?? 0) + 1);
    return m;
  }, [freeSpots]);
  const zones = inventory.zones.filter(
    (z) => z.assignment_mode !== 'quantity' && !!z.map_id && z.spot_count > 0,
  );
  const spotOptions = freeSpots.map((s) => ({
    value: s.spot_id,
    label: `${s.label} · ${inventory.zoneName(s.zone_id)}`,
  }));
  // The tapped spot stays selectable even if it was just taken: the quote says so.
  if (values.spotId && !spotOptions.some((o) => o.value === values.spotId)) {
    const s = inventory.spotById.get(values.spotId);
    if (s)
      spotOptions.unshift({ value: s.id, label: `${s.label} · ${inventory.zoneName(s.zone_id)}` });
  }

  const quoteReq = toQuoteRequest(effective);
  const quoteKey = quoteReq ? JSON.stringify(quoteReq) : null;
  const debouncedKey = useDebounced(quoteKey, 200);
  const quote = useDailySaleQuote(
    establishmentId,
    debouncedKey ? (JSON.parse(debouncedKey) as typeof quoteReq) : null,
  );
  const stale = quote.isFetching || debouncedKey !== quoteKey;
  const total = quote.data?.total;
  const available = quote.data?.available ?? false;
  const change = total !== undefined ? changeDue(total, values.tendered) : undefined;
  const openSession = session.data ?? null;
  const cashBlocked = !canCash || (session.isSuccess && !openSession);
  const ready = !!quoteReq && !!quote.data && available && !stale && !quote.error;

  const sell = async (method: TicketPaymentMethod) => {
    if (!ready || create.isPending) return;
    if (method === 'cash' && cashBlocked) return;
    if (!navigator.onLine) {
      setFailure(t('sale.offline'));
      return;
    }
    const body = toCreateRequest(effective, method, {
      cashSessionId: openSession?.session.id,
      expectedTotal: total,
    });
    if (!body) return;
    setFailure(null);
    let result: DailySaleResult;
    try {
      result = await create.mutateAsync({ body, idempotencyKey: keyRef.current.for(body) });
    } catch (err) {
      setFailure(problemText(err));
      return;
    }
    keyRef.current.reset();
    setSold(result);
    onSold?.(result);
    toast.success(t('sale.done', { number: result.ticket.number_label }));
    if (settings.data?.auto_print ?? true) {
      try {
        printer.print(await fetchPrint(result.ticket.id));
      } catch {
        toast.warning(t('sale.printFailed', { number: result.ticket.number_label }));
      }
    }
  };

  const newSale = useCallback(() => {
    setSold(null);
    setFailure(null);
    setValues(saleDefaults(values.date, { slots: values.slots }));
  }, [values.date, values.slots]);

  // Desktop shortcuts (outside text fields; the sheet/page is the active surface).
  const onShortcut = useEffectEvent((sc: SaleShortcut) => {
    if (sold) return;
    if (sc === 'FULL' || sc === 'AM' || sc === 'PM') patch({ slots: sc });
    else if (sc === 'spot') patch({ where: 'spot' });
    else if (sc === 'zone') {
      if (zoneFirst) patch({ where: 'zone' });
    } else void sell(sc);
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const sc = shortcutFor(e);
      if (!sc) return;
      e.preventDefault();
      onShortcut(sc);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (sold) {
    return (
      <>
        <SaleDone
          result={sold}
          zoneName={name(sold.ticket.zone_name)}
          onPrint={() => setPrintOpen(true)}
          onNew={newSale}
          onClose={onClose}
        />
        <TicketPrintDialog
          establishmentId={establishmentId}
          ticketId={sold.ticket.id}
          open={printOpen}
          onOpenChange={setPrintOpen}
          defaultFormat={settings.data?.print_format}
          mode="reprint"
        />
        {printer.area}
      </>
    );
  }

  const where: SaleWhere = zoneFirst ? values.where : 'spot';
  const loading = packList.isPending || inventory.isLoading;

  const summary = (
    <section
      aria-label={t('sale.summary')}
      className={cn(
        'space-y-3',
        layout === 'sheet'
          ? 'sticky bottom-0 -mx-4 border-t bg-background p-4 sm:-mx-6 sm:px-6'
          : // Phones: total and payment stay in view at the bottom of the screen.
            'sticky bottom-0 z-10 rounded-lg border bg-card p-4 shadow-md lg:top-4 lg:bottom-auto lg:shadow-none',
      )}
    >
      <div className="flex items-baseline justify-between gap-3">
        <p className="text-sm font-medium text-muted-foreground">{t('sale.total')}</p>
        <p
          className={cn('text-3xl font-bold tabular-nums', stale && 'opacity-60')}
          aria-live="polite"
          data-testid="sale-total"
        >
          {total !== undefined ? formatCents(total) : '—'}
        </p>
      </div>
      {quote.data && quote.data.breakdown.vat.length > 0 && (
        <p className="text-right text-xs text-muted-foreground">
          {quote.data.breakdown.vat
            .map((v) =>
              t('sale.vatIncluded', {
                rate: vatRateLabel(v.rate, locale),
                tax: formatCents(v.tax),
              }),
            )
            .join(' · ')}
        </p>
      )}
      {quote.data && where === 'zone' && quote.data.zone_free !== undefined && (
        <p className="text-sm text-muted-foreground">
          {t('sale.zoneFree', { count: quote.data.zone_free })}
        </p>
      )}
      {quote.data && !available && !stale && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertDescription>
            {where === 'zone' ? t('sale.zoneFull') : t('sale.spotTaken')}
          </AlertDescription>
        </Alert>
      )}
      {quote.error && !stale && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertDescription>{problemText(quote.error)}</AlertDescription>
        </Alert>
      )}
      {!quoteReq && !loading && (
        <p className="text-sm text-muted-foreground">
          {where === 'zone' ? t('sale.pickZone') : t('sale.pickSpot')}
        </p>
      )}
      {failure && (
        <Alert variant="destructive" role="alert">
          <CircleAlertIcon aria-hidden="true" />
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      )}
      {canCash && session.isSuccess && !openSession && (
        <Alert>
          <WalletIcon aria-hidden="true" />
          <AlertTitle>{t('sale.noSessionTitle')}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center gap-2">
            <span>{t('sale.noSession')}</span>
            <Button size="sm" variant="outline" asChild>
              <Link to="/e/$establishmentId/cassa" params={{ establishmentId }}>
                {t('sale.openCash')}
              </Link>
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {!cashBlocked && (
        <div className="grid grid-cols-2 items-end gap-3">
          <div className="space-y-1.5">
            <Label htmlFor={`${ids}-tendered`}>{t('sale.tendered')}</Label>
            <MoneyInput
              id={`${ids}-tendered`}
              value={values.tendered ?? null}
              onChange={(v) => patch({ tendered: v ?? undefined })}
            />
          </div>
          <p className="pb-2 text-right text-sm" aria-live="polite">
            {change !== undefined && (
              <>
                {t('sale.change')}{' '}
                <span className="text-lg font-semibold tabular-nums">{formatCents(change)}</span>
              </>
            )}
          </p>
        </div>
      )}
      <div className="grid grid-cols-2 gap-3">
        <PayButton
          method="cash"
          icon={<BanknoteIcon aria-hidden="true" />}
          label={t('method.cash')}
          disabled={!ready || cashBlocked || create.isPending}
          pending={create.isPending && create.variables?.body.payment.method === 'cash'}
          onClick={() => void sell('cash')}
        />
        <PayButton
          method="card"
          icon={<CreditCardIcon aria-hidden="true" />}
          label={t('method.card')}
          disabled={!ready || create.isPending}
          pending={create.isPending && create.variables?.body.payment.method === 'card'}
          onClick={() => void sell('card')}
        />
      </div>
      <p className="hidden text-xs text-muted-foreground lg:block">{t('sale.shortcutsHint')}</p>
    </section>
  );

  return (
    <div
      className={cn(
        layout === 'page' && 'grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]',
        layout === 'sheet' && 'space-y-6',
      )}
    >
      <div className="min-w-0 space-y-6">
        <fieldset className="space-y-3">
          <legend className="mb-2 text-sm font-medium">{t('sale.where')}</legend>
          <div className="flex flex-wrap items-center gap-3">
            {zoneFirst && (
              <SegmentedControl
                label={t('sale.where')}
                value={where}
                onValueChange={(v) => patch({ where: v as SaleWhere })}
                options={[
                  {
                    value: 'spot',
                    label: <ShortcutLabel text={t('where.spot')} shortcut="spot" />,
                  },
                  {
                    value: 'zone',
                    label: <ShortcutLabel text={t('where.zone')} shortcut="zone" />,
                    disabled: zones.length === 0,
                  },
                ]}
              />
            )}
            <div className="w-full sm:w-44">
              <DatePicker
                aria-label={t('sale.date')}
                value={values.date}
                onChange={(d) => d && patch({ date: d })}
              />
            </div>
          </div>
          {loading ? (
            <Skeleton className="h-11 w-full" />
          ) : where === 'spot' ? (
            <div className="space-y-1.5">
              <Label id={`${ids}-spot`}>{t('sale.spot')}</Label>
              <OptionSelect
                aria-labelledby={`${ids}-spot`}
                value={values.spotId}
                onChange={(v) => patch({ spotId: v })}
                placeholder={
                  freeSpots.length === 0 && day.isSuccess
                    ? t('sale.noFreeSpots')
                    : t('sale.spotPlaceholder')
                }
                options={spotOptions}
                className="h-11 w-full"
              />
            </div>
          ) : (
            <div
              role="radiogroup"
              aria-label={t('sale.zone')}
              className="grid grid-cols-2 gap-2 sm:grid-cols-3"
            >
              {zones.map((z) => {
                const free = freeByZone.get(z.id) ?? 0;
                const checked = values.zoneId === z.id;
                return (
                  <Button
                    key={z.id}
                    role="radio"
                    aria-checked={checked}
                    variant={checked ? 'default' : 'outline'}
                    className="h-auto min-h-14 flex-col items-start gap-0.5 whitespace-normal py-2 text-left"
                    onClick={() => patch({ zoneId: z.id })}
                  >
                    <span className="font-semibold">{name(z.name) || z.code}</span>
                    <span
                      className={cn('text-xs', checked ? 'opacity-90' : 'text-muted-foreground')}
                    >
                      {t('sale.freeCount', { count: free })}
                    </span>
                  </Button>
                );
              })}
            </div>
          )}
          {where === 'zone' && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <MapPinIcon aria-hidden="true" className="size-3.5" />
              {t('sale.zoneHint')}
            </p>
          )}
        </fieldset>

        <div className="space-y-2">
          <Label id={`${ids}-slots`}>{t('sale.slots')}</Label>
          <ToggleGroup
            type="single"
            variant="outline"
            aria-labelledby={`${ids}-slots`}
            value={values.slots}
            onValueChange={(s) => s && patch({ slots: s as SaleSlots })}
            className="grid w-full grid-cols-3"
          >
            {SALE_SLOTS.map((s) => (
              <ToggleGroupItem key={s} value={s} className="h-12">
                <ShortcutLabel text={t(`slots.${s}`)} shortcut={s} />
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>

        {packs.length > 1 && (
          <div className="space-y-2">
            <Label id={`${ids}-pack`}>{t('sale.pack')}</Label>
            <ToggleGroup
              type="single"
              variant="outline"
              aria-labelledby={`${ids}-pack`}
              value={packId}
              onValueChange={(p) => p && patch({ packId: p })}
              className="flex w-full flex-wrap justify-start"
            >
              {packs.map((p) => (
                <ToggleGroupItem key={p.id} value={p.id} className="h-12 flex-none px-4">
                  {name(p.name) || p.code}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
        )}

        {extras.length > 0 && (
          <SaleExtras extras={extras} values={values} name={name} onChange={patch} />
        )}

        <fieldset className="space-y-2">
          <legend className="mb-2 text-sm font-medium">
            {t('sale.holder')}{' '}
            <span className="font-normal text-muted-foreground">{t('sale.optional')}</span>
          </legend>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor={`${ids}-last`}>{t('sale.lastName')}</Label>
              <Input
                id={`${ids}-last`}
                autoComplete="off"
                maxLength={100}
                value={values.lastName}
                onChange={(e) => patch({ lastName: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`${ids}-first`}>{t('sale.firstName')}</Label>
              <Input
                id={`${ids}-first`}
                autoComplete="off"
                maxLength={100}
                value={values.firstName}
                onChange={(e) => patch({ firstName: e.target.value })}
              />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">{t('sale.holderHint')}</p>
        </fieldset>
      </div>
      {summary}
      {printer.area}
    </div>
  );
}

function ShortcutLabel({ text, shortcut }: { text: string; shortcut: SaleShortcut }) {
  return (
    <>
      {text}
      <Kbd aria-hidden="true" className="ml-1 hidden lg:inline-flex">
        {shortcutKey(shortcut)}
      </Kbd>
    </>
  );
}

function PayButton({
  method,
  icon,
  label,
  disabled,
  pending,
  onClick,
}: {
  method: TicketPaymentMethod;
  icon: ReactNode;
  label: string;
  disabled: boolean;
  pending: boolean;
  onClick: () => void;
}) {
  return (
    <Button
      size="lg"
      variant={method === 'cash' ? 'default' : 'secondary'}
      className="h-14 text-base"
      disabled={disabled}
      onClick={onClick}
      data-method={method}
    >
      {pending ? <Spinner /> : icon}
      {label}
      <Kbd aria-hidden="true" className="ml-1 hidden lg:inline-flex">
        {shortcutKey(method)}
      </Kbd>
    </Button>
  );
}

type Extra = NonNullable<ReturnType<typeof extrasResource.useList>['data']>[number];

function SaleExtras({
  extras,
  values,
  name,
  onChange,
}: {
  extras: Extra[];
  values: SaleValues;
  name: (t: Record<string, string> | undefined) => string;
  onChange: (p: Partial<SaleValues>) => void;
}) {
  const { t } = useTranslation('dailySale');
  const unitText = useExtraUnitText();
  const maxText = useQuantityMaxText();
  return (
    <ExtraList title={t('sale.extras')}>
      {extras.map((x) => {
        const label = name(x.name) || x.code;
        return (
          <ExtraRow key={x.id} name={label} detail={unitText(x)} hint={maxText(x.max_quantity)}>
            <QuantityStepper
              label={label}
              size="lg"
              min={0}
              max={x.max_quantity}
              value={values.extras[x.id] ?? 0}
              onChange={(q) => onChange({ extras: { ...values.extras, [x.id]: q } })}
            />
          </ExtraRow>
        );
      })}
    </ExtraList>
  );
}

function SaleDone({
  result,
  zoneName,
  onPrint,
  onNew,
  onClose,
}: {
  result: DailySaleResult;
  zoneName: string;
  onPrint: () => void;
  onNew: () => void;
  onClose?: () => void;
}) {
  const { t } = useTranslation('dailySale');
  const { ticket } = result;
  const newRef = useRef<HTMLButtonElement>(null);
  useEffect(() => newRef.current?.focus(), []);
  return (
    <section
      aria-labelledby="sale-done-title"
      className="mx-auto max-w-md space-y-4 rounded-lg border bg-card p-6 text-center"
    >
      <TicketCheckIcon aria-hidden="true" className="mx-auto size-10 text-success" />
      <h2 id="sale-done-title" className="text-lg font-semibold">
        {t('done.title')}
      </h2>
      <p className="text-5xl font-bold tabular-nums" data-testid="ticket-number">
        {ticket.number_label}
      </p>
      <p className="text-sm text-muted-foreground">
        {ticket.spot_label
          ? t('done.spot', { label: ticket.spot_label })
          : t('done.unassigned', { zone: zoneName })}
      </p>
      <Separator />
      <div className="flex items-baseline justify-between text-sm">
        <span>{t('sale.total')}</span>
        <span className="text-lg font-semibold tabular-nums">
          {formatCents(ticket.total_amount)}
        </span>
      </div>
      {result.change_amount !== undefined && result.change_amount > 0 && (
        <div className="flex items-baseline justify-between text-sm" aria-live="polite">
          <span>{t('sale.change')}</span>
          <span className="text-2xl font-bold tabular-nums">
            {formatCents(result.change_amount)}
          </span>
        </div>
      )}
      <div className="grid gap-2 sm:grid-cols-2">
        <Button variant="outline" size="lg" onClick={onPrint}>
          <PrinterIcon aria-hidden="true" />
          {t('actions.reprint')}
        </Button>
        <Button ref={newRef} size="lg" onClick={onNew}>
          <RotateCcwIcon aria-hidden="true" />
          {t('done.new')}
        </Button>
      </div>
      {onClose && (
        <Button variant="ghost" className="w-full" onClick={onClose}>
          {t('common.close', { ns: 'translation' })}
        </Button>
      )}
    </section>
  );
}
