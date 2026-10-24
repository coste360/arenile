import { cn } from '@arenile/shared/lib/utils';
import { useMemo } from 'react';
import { encodeQr, qrPath } from './qr';

export interface QrCodeProps {
  /** Text carried by the code (the booking code of the ticket). */
  value: string;
  /** Accessible name; the code is decorative without it. */
  label?: string;
  className?: string;
}

/**
 * QR code as an inline SVG drawn in the current text colour (black on the
 * receipt paper), with the standard 4-module quiet zone. Falls back to
 * nothing when the payload does not fit (the code is printed in clear too).
 */
export function QrCode({ value, label, className }: QrCodeProps) {
  const drawing = useMemo(() => {
    try {
      const m = encodeQr(value);
      return { size: m.length + 8, d: qrPath(m) };
    } catch {
      return null;
    }
  }, [value]);
  if (!drawing) return null;
  return (
    <svg
      viewBox={`0 0 ${drawing.size} ${drawing.size}`}
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      shapeRendering="crispEdges"
      className={cn('fill-current', className)}
      data-qr={value}
    >
      <path d={drawing.d} />
    </svg>
  );
}
