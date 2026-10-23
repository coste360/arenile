import { usePanelMode, type PanelMode } from '../../lib/meta';
import { pairingBlock, type PairingBlock } from './lib';

export interface PairingAvailability {
  mode: PanelMode;
  /** Why "Abbina un dispositivo" is disabled, or null when it is available. */
  block: PairingBlock;
}

/**
 * Desktop-mode detection for the devices page (spec 29.1, 29.2): on the
 * desktop edition in local mode no other device can reach this instance,
 * so pairing needs the LAN mode first; on a server instance, in LAN mode
 * and in the remote mode of the desktop app (which shows a server panel)
 * pairing is available.
 */
export function usePairingAvailability(): PairingAvailability {
  const mode = usePanelMode();
  return { mode, block: pairingBlock(mode) };
}
