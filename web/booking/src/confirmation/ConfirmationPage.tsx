export interface ConfirmationPageProps {
  slug: string;
  session: { bookingId: string; token: string };
}

/**
 * Confirmation (spec 12.3): polls the checkout state after the payment
 * return until `done` (never confirms by itself, INV-19), then shows code,
 * QR (`confirmation.qr_svg`), summary, "aggiungi al calendario" (.ics via
 * fetch + blob with the session header) and the masked email that received
 * the manage link; posts `arenile:booked` with the code to the host page.
 * Owner: booking app builder.
 */
export function ConfirmationPage(_props: ConfirmationPageProps) {
  return null;
}
