import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Report" (spec 15.3, M5A). One page with a period toolbar (from/to,
 * presets: this season, last 30 days, month; compare with previous year
 * switch) and NavTabs: Riepilogo (StatCards from reports/summary + year
 * comparison deltas), Occupazione (by day/zone/row/slot), Ricavi (basis
 * collected/booked, group by period/channel/pack/extra/method/VAT),
 * Prenotazioni (channel, lead time, formula), Cancellazioni e no-show,
 * Abbonamenti e rilasci, Clienti migliori, Saldi aperti, Confronto anni.
 * Every tab: DataTable + "Esporta CSV" (plain link to the same endpoint
 * with format=csv, like CorrispettiviPage). Charts only with shadcn chart
 * components on theme tokens; must work at 390px (tables scroll inside
 * their card, never the page). Permission reports.read. API: api.ts.
 * Owner: reports frontend builder.
 */
export function ReportsPage() {
  return (
    <PlaceholderPage
      titleKey="nav.reports"
      descriptionKey="reports:page.description"
      milestone="M5"
    />
  );
}
