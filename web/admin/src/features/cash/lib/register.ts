import type { TFunction } from 'i18next';

/** Registers with a translated name; others (custom desks) show their code. */
const KNOWN_REGISTERS = ['main'] as const;

/** Human name of a cash register code ("main" -> "Cassa principale"). */
export function registerLabel(t: TFunction, code: string): string {
  return (KNOWN_REGISTERS as readonly string[]).includes(code) ? t(`registers.${code}`) : code;
}
