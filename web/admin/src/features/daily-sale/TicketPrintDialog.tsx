import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@arenile/shared/ui/dialog';
import { QueryState, SegmentedControl } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CircleAlertIcon, PrinterIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment } from '../../establishment';
import { useReprintTicket, useTicketPrint, type TicketPrintFormat } from './api';
import { PRINT_FORMATS } from './lib';
import { useTicketOutput } from './printing';
import { TicketPrint } from './TicketPrint';

export interface TicketPrintDialogProps {
  establishmentId: string;
  ticketId: string | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Format shown first (default: the establishment setting returned by the server). */
  defaultFormat?: TicketPrintFormat;
  /**
   * `first`: print right after the sale (no count); `reprint` (default):
   * counted reprint with audit, marked "Ristampa" on the paper.
   */
  mode?: 'first' | 'reprint';
}

/**
 * Print preview of a ticket: pick thermal 80/58 mm, A5 or A4, see the paper
 * as it will come out, then print (or reprint) on the thermal printer when
 * the daily_ticket route is ready (ADR 0021), through the browser otherwise.
 */
export function TicketPrintDialog({
  establishmentId,
  ticketId,
  open,
  onOpenChange,
  defaultFormat,
  mode = 'reprint',
}: TicketPrintDialogProps) {
  const { t } = useTranslation('dailySale');
  const problemText = useProblemText();
  const { establishment } = useCurrentEstablishment();
  const [format, setFormat] = useState<TicketPrintFormat | undefined>(defaultFormat);
  const preview = useTicketPrint(establishmentId, open ? ticketId : undefined, format);
  const reprint = useReprintTicket(establishmentId);
  const printer = useTicketOutput();
  const shown = format ?? preview.data?.format ?? 'thermal_80';

  const doPrint = async () => {
    if (!ticketId || !preview.data) return;
    const shownData = { ...preview.data, format: shown };
    // A5/A4 are sheets for an office printer: always the browser.
    const sheet = shown === 'a4' || shown === 'a5';
    if (mode === 'first') {
      await printer.print(ticketId, () => shownData, undefined, sheet);
      return;
    }
    // Counted reprint first (audit, "Ristampa"), then the thermal printer
    // when routed (receipt formats) or the browser in the chosen format.
    const data = await reprint.mutateAsync({ ticketId, format: shown });
    await printer.print(ticketId, () => data, undefined, sheet);
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          closeLabel={t('common.close', { ns: 'translation' })}
          className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        >
          <DialogHeader>
            <DialogTitle>
              {preview.data
                ? t('printDialog.title', { number: preview.data.ticket.number_label })
                : t('printDialog.titleLoading')}
            </DialogTitle>
            <DialogDescription>{t('printDialog.description')}</DialogDescription>
          </DialogHeader>
          <SegmentedControl
            label={t('printDialog.format')}
            value={shown}
            onValueChange={(v) => setFormat(v as TicketPrintFormat)}
            options={PRINT_FORMATS.map((f) => ({ value: f, label: t(`format.${f}`) }))}
            collapseOnNarrow
          />
          {printer.thermal && (
            <p className="text-sm text-muted-foreground">{t('printDialog.thermalHint')}</p>
          )}
          <QueryState
            isLoading={preview.isPending && !!ticketId}
            error={preview.error}
            onRetry={() => void preview.refetch()}
            rows={6}
          />
          {preview.data && (
            <div className="flex justify-center overflow-x-auto rounded-lg border bg-muted p-3">
              <TicketPrint
                data={{ ...preview.data, format: shown }}
                timeZone={establishment?.timezone}
                className="shadow-sm"
              />
            </div>
          )}
          {reprint.error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{problemText(reprint.error)}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common.close', { ns: 'translation' })}
            </Button>
            <Button
              onClick={() => void doPrint().catch(() => undefined)}
              disabled={!preview.data || reprint.isPending}
            >
              {reprint.isPending ? <Spinner /> : <PrinterIcon aria-hidden="true" />}
              {mode === 'first' ? t('actions.print') : t('actions.reprint')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {printer.area}
    </>
  );
}
