import { cn } from '@arenile/shared/lib/utils';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

export interface ExtraListProps {
  /** Legend of the group ("Extra"). */
  title: ReactNode;
  children: ReactNode;
  className?: string;
}

/** Bordered list of ExtraRow items under a legend (same look in the panel and on the public page). */
export function ExtraList({ title, children, className }: ExtraListProps) {
  return (
    <fieldset className={cn('min-w-0 space-y-2', className)}>
      <legend className="mb-2 text-sm font-medium">{title}</legend>
      <ul className="divide-y rounded-lg border bg-card">{children}</ul>
    </fieldset>
  );
}

export interface ExtraRowProps {
  /** Name of the extra: one line, truncated, full text in the tooltip. */
  name: string;
  /** Muted second line: unit and price ("per postazione · al giorno · 5,00 €"). */
  detail?: ReactNode;
  /** Short helper under the control ("max 2"). */
  hint?: ReactNode;
  /** Error under the row (client validation or a server field error). */
  error?: ReactNode;
  /** Id of the error/hint text, for the control's aria-describedby. */
  messageId?: string;
  /** The quantity control (a QuantityStepper), aligned right at a fixed width. */
  children: ReactNode;
  className?: string;
}

/**
 * One extra in an ExtraList: name and detail on the left (they shrink and
 * truncate), the quantity control and its hint on the right (never shrink),
 * the error on a full-width line below. Fits 320 px without overflow.
 */
export function ExtraRow({
  name,
  detail,
  hint,
  error,
  messageId,
  children,
  className,
}: ExtraRowProps) {
  return (
    <li
      data-invalid={error ? true : undefined}
      className={cn('flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5', className)}
    >
      <div className="min-w-0 flex-1 basis-0">
        <p className="truncate text-sm font-medium" title={name}>
          {name}
        </p>
        {detail && <p className="truncate text-xs text-muted-foreground">{detail}</p>}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        {children}
        {hint && !error && (
          <p id={messageId} className="text-xs text-muted-foreground">
            {hint}
          </p>
        )}
      </div>
      {error && (
        <p id={messageId} role="alert" className="basis-full text-sm text-destructive">
          {error}
        </p>
      )}
    </li>
  );
}

const UNIT_KEYS: Record<string, string | undefined> = {
  per_day: 'ui.extra.perDay',
  per_slot: 'ui.extra.perSlot',
};

/**
 * Text of an extra's unit for ExtraRow `detail`: what one unit covers
 * ("per postazione" / "per prenotazione") and how it is charged ("al giorno").
 */
export function useExtraUnitText() {
  const { t } = useTranslation();
  return (extra: { attach_to: string; pricing_unit: string }) => {
    const unit = UNIT_KEYS[extra.pricing_unit];
    return [
      t(extra.attach_to === 'line' ? 'ui.extra.perSpot' : 'ui.extra.perBooking'),
      ...(unit ? [t(unit)] : []),
    ].join(' · ');
  };
}

/** "max 2" helper shown under a QuantityStepper. */
export function useQuantityMaxText() {
  const { t } = useTranslation();
  return (max: number) => t('ui.quantity.max', { max });
}
