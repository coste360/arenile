import type { TFunction } from 'i18next';

/** Registers with a translated name; others (custom desks) show their code. */
const KNOWN_REGISTERS = ['main'] as const;

/** Human name of a cash register code ("main" -> "Cassa principale"). */
export function registerLabel(t: TFunction, code: string): string {
  return (KNOWN_REGISTERS as readonly string[]).includes(code) ? t(`registers.${code}`) : code;
}

/** Name of the register of a session: its configured name, else the code label. */
export function sessionRegisterLabel(
  t: TFunction,
  session: { register: string; register_name?: string },
): string {
  return session.register_name || registerLabel(t, session.register);
}

const STORAGE_PREFIX = 'arenile.cash.register.';

/**
 * Register last used on this device for an establishment (a till PC stays
 * on its register). Per-device convenience only: storage may be missing.
 */
export function readPreferredRegister(establishmentId: string): string {
  try {
    return window.localStorage.getItem(STORAGE_PREFIX + establishmentId) ?? '';
  } catch {
    return '';
  }
}

export function writePreferredRegister(establishmentId: string, registerId: string): void {
  try {
    window.localStorage.setItem(STORAGE_PREFIX + establishmentId, registerId);
  } catch {
    // Private mode or blocked storage: the choice is just not remembered.
  }
}

/**
 * Register to show: the preferred one when still active, else the default
 * one, else the first. Empty when there are no registers yet.
 */
export function pickRegister(
  registers: readonly { id: string; is_default: boolean }[],
  preferred: string,
): string {
  if (preferred && registers.some((r) => r.id === preferred)) return preferred;
  return (registers.find((r) => r.is_default) ?? registers[0])?.id ?? '';
}
