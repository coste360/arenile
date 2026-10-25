import type { Schemas } from '@arenile/shared/api';

/** The six steps of the guided setup (spec 15.2). */
export const STEPS = [
  'details',
  'template',
  'rows',
  'pricelist',
  'publicPage',
  'payments',
] as const;
export type StepId = (typeof STEPS)[number];
export type StepState = 'pending' | 'done' | 'skipped';
export type TemplateCode = Schemas['SetupTemplateCode'];
export type SetupStatus = Schemas['SetupStatus'];

export const TEMPLATE_CODES: readonly TemplateCode[] = [
  'classic_rows',
  'quantity_beach',
  'lido_with_pool',
  'empty',
];

export interface WizardProgress {
  /** Schema version of the stored progress. */
  v: 1;
  current: number;
  states: Record<StepId, StepState>;
  template?: TemplateCode;
  /** Season and map created by apply-template (used by the quick price list). */
  seasonId?: string;
  mapId?: string;
  finished: boolean;
}

export type WizardAction =
  | { type: 'complete' }
  | { type: 'skip' }
  | { type: 'back' }
  | { type: 'goto'; step: number }
  | { type: 'setTemplate'; template: TemplateCode }
  | { type: 'applied'; seasonId?: string; mapId?: string }
  | { type: 'restart' }
  /** The establishment was reset: every step is pending again. */
  | { type: 'reset' };

function allPending(): Record<StepId, StepState> {
  return Object.fromEntries(STEPS.map((s) => [s, 'pending'])) as Record<StepId, StepState>;
}

function advance(p: WizardProgress): WizardProgress {
  if (p.current >= STEPS.length - 1) return { ...p, finished: true };
  return { ...p, current: p.current + 1 };
}

/**
 * Initial progress. Without a saved progress the server status tells which
 * steps were already done (e.g. the template applied from another device),
 * and the wizard starts at the first one still pending.
 */
export function initialProgress(status?: SetupStatus): WizardProgress {
  const states = allPending();
  if (status) {
    if (status.has_season || status.has_map || status.has_catalog) {
      states.template = 'done';
      states.rows = 'done';
    }
    if (status.has_prices) states.pricelist = 'done';
    if (status.booking_enabled) states.publicPage = 'done';
    // Something configured means the establishment data was already entered at creation.
    if (states.template === 'done') states.details = 'done';
  }
  const first = STEPS.findIndex((s) => states[s] === 'pending');
  return { v: 1, current: first < 0 ? 0 : first, states, finished: false };
}

export function wizardReducer(p: WizardProgress, action: WizardAction): WizardProgress {
  const step = STEPS[p.current] as StepId;
  switch (action.type) {
    case 'complete':
      return advance({ ...p, states: { ...p.states, [step]: 'done' } });
    case 'skip':
      // A step done earlier stays done when the user passes over it again.
      return advance({
        ...p,
        states: { ...p.states, [step]: p.states[step] === 'done' ? 'done' : 'skipped' },
      });
    case 'back':
      return { ...p, current: Math.max(0, p.current - 1), finished: false };
    case 'goto':
      if (action.step < 0 || action.step >= STEPS.length) return p;
      return { ...p, current: action.step, finished: false };
    case 'setTemplate':
      return { ...p, template: action.template };
    case 'applied':
      return {
        ...p,
        seasonId: action.seasonId ?? p.seasonId,
        mapId: action.mapId ?? p.mapId,
      };
    case 'restart':
      return { ...p, current: 0, finished: false };
    case 'reset':
      return initialProgress();
    default:
      return p;
  }
}

/** Number of steps done or skipped (for the progress bar). */
export function stepsHandled(p: WizardProgress): number {
  return STEPS.filter((s) => p.states[s] !== 'pending').length;
}

// ---- Persistence ---------------------------------------------------------------------
// Progress is per establishment and per browser. The server keeps what matters
// (season, map, prices, booking flag): on another device the wizard resumes
// from the server status instead.

export const storageKey = (establishmentId: string) => `arenile.setup.${establishmentId}`;

function isProgress(value: unknown): value is WizardProgress {
  if (!value || typeof value !== 'object') return false;
  const p = value as Partial<WizardProgress>;
  if (p.v !== 1 || typeof p.current !== 'number' || typeof p.finished !== 'boolean') return false;
  if (p.current < 0 || p.current >= STEPS.length) return false;
  if (!p.states || typeof p.states !== 'object') return false;
  return STEPS.every((s) => ['pending', 'done', 'skipped'].includes((p.states as never)[s]));
}

export function loadProgress(storage: Storage | undefined, establishmentId: string) {
  try {
    const raw = storage?.getItem(storageKey(establishmentId));
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return isProgress(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function saveProgress(
  storage: Storage | undefined,
  establishmentId: string,
  progress: WizardProgress,
): void {
  try {
    storage?.setItem(storageKey(establishmentId), JSON.stringify(progress));
  } catch {
    // Storage full or disabled (private mode): progress simply is not kept.
  }
}

/** localStorage when available (it throws in some sandboxed iframes). */
export function browserStorage(): Storage | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}
