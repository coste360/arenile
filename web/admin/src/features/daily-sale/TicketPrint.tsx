import { formatInstant, formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { cn } from '@arenile/shared/lib/utils';
import { ArenileAppIcon } from '@arenile/shared/ui/patterns';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { localize } from '../configuration/lib/i18nText';
import type { TicketPrintData, TicketPrintFormat } from './api';
import { vatRateLabel } from './lib';
import { QrCode } from './QrCode';

export interface TicketPrintProps {
  data: TicketPrintData;
  /** UI language for names and dates (default the i18n language). */
  locale?: string;
  /** Establishment time zone of the issue time (default Europe/Rome). */
  timeZone?: string;
  className?: string;
}

/**
 * Paper width of each format. Thermal widths are the printable area of the
 * roll (72 mm on 80 mm paper, 48 mm on 58 mm); sheets keep page margins.
 */
const FORMAT_CLASSES: Record<TicketPrintFormat, string> = {
  thermal_80: 'w-[72mm] max-w-full p-2 font-mono text-xs',
  thermal_58: 'w-[48mm] max-w-full p-1 font-mono text-xs',
  a5: 'w-full max-w-[128mm] p-6 text-sm',
  a4: 'w-full max-w-[180mm] p-8 text-base',
};

function Line({
  label,
  children,
  strong,
}: {
  label: ReactNode;
  children: ReactNode;
  strong?: boolean;
}) {
  return (
    <div
      className={cn('flex flex-wrap items-baseline justify-between gap-x-3', strong && 'font-bold')}
    >
      <dt className="min-w-0">{label}</dt>
      <dd className="ml-auto text-right tabular-nums">{children}</dd>
    </div>
  );
}

function Rule() {
  return <hr className="my-2 border-dashed border-current" />;
}

/**
 * Printable ticket (thermal 80/58 mm, A5, A4) styled only with Tailwind:
 * establishment, number and date, spot or zone ("da assegnare"), slot,
 * pack, extras, total with the VAT breakdown, payment method, QR of the
 * booking code and a small Arenile mark. Always black on white (paper), in
 * the preview as on the printer, whatever the colour scheme.
 */
export function TicketPrint({ data, locale, timeZone, className }: TicketPrintProps) {
  const { t, i18n } = useTranslation('dailySale');
  const lang = locale ?? i18n.language ?? 'it';
  const name = (text: Record<string, string> | undefined) => localize(text, lang, 'it');
  const { ticket, establishment } = data;
  const sheet = data.format === 'a4' || data.format === 'a5';
  const voided = ticket.status === 'voided';
  const place = ticket.spot_label
    ? t('print.spot', { label: ticket.spot_label })
    : t('print.zoneUnassigned', { zone: name(ticket.zone_name) });

  return (
    <article
      aria-label={t('print.label', { number: ticket.number_label })}
      data-format={data.format}
      className={cn(
        // Paper: fixed black on white, independent of the light/dark theme.
        'mx-auto bg-white text-black',
        FORMAT_CLASSES[data.format],
        className,
      )}
    >
      <header className="flex flex-col items-center gap-1 text-center">
        {establishment.logo_url && (
          <img
            src={establishment.logo_url}
            alt=""
            className={cn('w-auto object-contain grayscale', sheet ? 'h-16' : 'h-10')}
          />
        )}
        <p className={cn('font-bold', sheet && 'text-lg')}>{establishment.name}</p>
        {(establishment.legal_name || establishment.vat_number) && (
          <p>
            {[
              establishment.legal_name,
              establishment.vat_number && t('print.vatNumber', { vat: establishment.vat_number }),
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>
        )}
        {establishment.address && <p>{establishment.address}</p>}
      </header>

      <Rule />

      <section className="text-center" aria-label={t('print.numberSection')}>
        <p className="uppercase">
          {t(ticket.kind === 'entry' ? 'print.entryTitle' : 'print.title')}
        </p>
        <p className={cn('font-bold tabular-nums', sheet ? 'text-4xl' : 'text-3xl')}>
          {ticket.number_label}
        </p>
        {ticket.kind === 'sale' && (
          // The line the guest and the staff read on the sand: as large as
          // the number, under it.
          <p className={cn('font-bold', sheet ? 'text-2xl' : 'text-xl')} data-testid="print-place">
            {place}
          </p>
        )}
        <p className="font-bold">
          {t('print.validOn', { date: formatIsoDate(ticket.ticket_date) })}
        </p>
        <p>
          {t('print.issuedAt', {
            time: formatInstant(data.issued_at, {
              locale: lang,
              timeZone: timeZone ?? 'Europe/Rome',
            }),
          })}
        </p>
        {data.reprint && <p className="font-bold uppercase">{t('print.reprint')}</p>}
        {voided && (
          <p className="mt-1 border-2 border-current py-1 font-bold uppercase">
            {t('print.voided')}
          </p>
        )}
      </section>

      <Rule />

      <dl className="space-y-0.5">
        {ticket.kind === 'sale' && (
          <>
            <Line label={t('print.slots')}>{t(`slots.${ticket.slots}`)}</Line>
            {ticket.pack_name && <Line label={t('print.pack')}>{name(ticket.pack_name)}</Line>}
          </>
        )}
        {ticket.holder_name && <Line label={t('print.holder')}>{ticket.holder_name}</Line>}
      </dl>

      {(ticket.extras.length > 0 || ticket.entry_items.length > 0) && (
        <>
          <Rule />
          <dl className="space-y-0.5">
            {ticket.entry_items.map((it) => (
              <Line key={it.entry_ticket_type_id} label={`${it.quantity} × ${name(it.name)}`}>
                {formatCents(it.total)}
              </Line>
            ))}
            {ticket.extras.map((x) => (
              <Line key={x.extra_id} label={`${x.quantity} × ${name(x.name)}`}>
                {formatCents(x.total)}
              </Line>
            ))}
          </dl>
        </>
      )}

      <Rule />

      <dl className="space-y-0.5">
        <Line label={t('print.total')} strong>
          <span className={sheet ? 'text-xl' : 'text-base'}>
            {formatCents(ticket.total_amount)}
          </span>
        </Line>
        {ticket.vat.map((v) => (
          <Line key={v.rate} label={t('print.vatLine', { rate: vatRateLabel(v.rate, lang) })}>
            {t('print.vatAmounts', { taxable: formatCents(v.taxable), tax: formatCents(v.tax) })}
          </Line>
        ))}
        <Line label={t('print.payment')}>{t(`method.${ticket.payment_method}`)}</Line>
      </dl>
      {/* The receipt of the sale is the fiscal document: this ticket is not one. */}
      <p className="mt-1 text-center font-bold uppercase" data-testid="print-non-fiscal">
        {t('print.nonFiscal')}
      </p>

      <Rule />

      <footer className="flex flex-col items-center gap-1 text-center">
        <QrCode
          value={data.qr_payload}
          label={t('print.qr', { code: ticket.booking_code })}
          className={sheet ? 'size-36' : 'size-28'}
        />
        <p className="tabular-nums">{ticket.booking_code}</p>
        {data.footer_text && <p>{data.footer_text}</p>}
        <p className="mt-1 flex items-center gap-1 opacity-70">
          <ArenileAppIcon className="size-3 grayscale" />
          <span className="font-mono">Arenile</span>
        </p>
      </footer>
    </article>
  );
}
