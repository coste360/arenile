import { useTranslation } from 'react-i18next';
import { ScaffoldPage } from '../../components/ScaffoldPage';

/**
 * Configurazione → Stampanti: ESC/POS network printers (listPrinters,
 * createPrinter, updatePrinter, deletePrinter, checkPrinter, testPrinter),
 * document routing per cash register (getPrintRoutes, updatePrintRoutes) and
 * recent print jobs (listPrintJobs). On the server edition
 * (PrinterList.escpos_available = false) the page explains that ESC/POS
 * printing needs the desktop app and that printing falls back to the browser.
 * Scaffold until the printers builder implements it (ADR 0021).
 */
export function PrintersPage() {
  const { t } = useTranslation('printers');
  return <ScaffoldPage title={t('page.title')} description={t('page.description')} />;
}
