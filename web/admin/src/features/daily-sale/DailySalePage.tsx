import { Button } from '@arenile/shared/ui/button';
import { Page, ResponsiveTabsList } from '@arenile/shared/ui/patterns';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { Link, useSearch } from '@tanstack/react-router';
import { ReceiptIcon, ScanLineIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';
import { EntrySalePanel } from '../entry-tickets';
import { isIsoDate, todayIn } from '../operations/lib/dates';
import { useDailySaleSettings, useFetchTicketPrint, type DailySaleResult } from './api';
import { DailySaleForm } from './DailySaleForm';
import { useTicketOutput } from './printing';

type SaleTab = 'spots' | 'entries';

/**
 * "Vendita rapida" (Cassa): the daily sale form as a page, for the cash desk
 * without the map. The same form opens as VenditaGiornalieraSheet from the
 * day map ("Vendi giornata" on a free spot). The "Ingressi" tab sells
 * entry-only tickets (spec 14.3) with the same numbering.
 */
export function DailySalePage() {
  const { t } = useTranslation('dailySale');
  const establishmentId = useRequiredEstablishmentId();
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  // `?date=` (e.g. a link from the map of another day); today by default.
  // `?tab=entries` opens the entry tickets directly.
  const search = useSearch({ strict: false }) as { date?: unknown; tab?: unknown };
  const date = isIsoDate(search.date) ? search.date : today;
  const [tab, setTab] = useState<SaleTab>(search.tab === 'entries' ? 'entries' : 'spots');
  const settings = useDailySaleSettings(establishmentId);
  const fetchPrint = useFetchTicketPrint(establishmentId);
  // Entry tickets print without a register: the establishment-wide route.
  const printer = useTicketOutput();

  const onEntrySold = async (result: DailySaleResult) => {
    if (!(settings.data?.auto_print ?? true)) return;
    const printFailed = () =>
      toast.warning(t('sale.printFailed', { number: result.ticket.number_label }));
    try {
      await printer.print(result.ticket.id, () => fetchPrint(result.ticket.id), printFailed);
    } catch {
      printFailed();
    }
  };

  return (
    <Page
      title={t('sale.title')}
      description={t('sale.description')}
      actions={
        <>
          <Button variant="outline" asChild>
            <Link to="/e/$establishmentId/ticket/varco" params={{ establishmentId }}>
              <ScanLineIcon aria-hidden="true" />
              {t('sale.gate')}
            </Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="/e/$establishmentId/ticket" params={{ establishmentId }}>
              <ReceiptIcon aria-hidden="true" />
              {t('tickets.title')}
            </Link>
          </Button>
        </>
      }
    >
      <Tabs value={tab} onValueChange={(v) => setTab(v as SaleTab)}>
        <ResponsiveTabsList
          label={t('sale.tabs.label')}
          value={tab}
          onValueChange={(v) => setTab(v as SaleTab)}
          options={[
            { value: 'spots', label: t('sale.tabs.spots') },
            { value: 'entries', label: t('sale.tabs.entries') },
          ]}
        />
        <TabsContent value="spots" className="mt-4">
          {/* Keyed by day: the form starts again from today's defaults after midnight. */}
          <DailySaleForm key={date} establishmentId={establishmentId} date={date} />
        </TabsContent>
        <TabsContent value="entries" className="mt-4">
          <EntrySalePanel key={date} date={date} onSold={(r) => void onEntrySold(r)} />
        </TabsContent>
      </Tabs>
      {printer.area}
    </Page>
  );
}
