import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Dispositivi" (Configurazione, spec 29.2): paired devices of the
 * establishment (name, user, role, platform, last use, fiscal bridge badge)
 * with revoke (AlertDialog), and "Abbina un dispositivo": creates a pairing
 * code and shows the QR (`qr_text`) with the code ABCD-EFGH and a countdown
 * to `expires_at`. Desktop edition without LAN mode: explain how to enable
 * it (409 lan_mode_required). Owners can pair a device for another user.
 * API: POST devices/pairing-codes, GET devices, DELETE devices/{deviceId},
 * GET fiscal-bridge. Must work at 390px. Owner: devices builder.
 */
export function DevicesPage() {
  return (
    <PlaceholderPage titleKey="devices:title" descriptionKey="devices:description" milestone="M6" />
  );
}
