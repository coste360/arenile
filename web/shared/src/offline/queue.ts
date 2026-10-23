import { ApiProblem } from '../api/client';
import type { KeyValueStore } from './kv';

/*
 * Persistent FIFO of operations recorded while offline (spec 21.2).
 *
 * Lifecycle of an operation:
 *
 *   enqueue ──► pending ──flush──► sending ──2xx──► (removed)
 *                  ▲                  │
 *                  │  network / 5xx / 429 → pending, flush stops (order kept)
 *                  │  401             → pending, queue paused until resume()
 *                  │  409 / 412 / 422 → conflict (shown to the user), flush goes on
 *                  │  other 4xx       → failed   (shown to the user), flush goes on
 *                  └──── retry(id): conflict|failed → pending with a NEW key
 *
 * Safety of replays: every operation carries the Idempotency-Key generated
 * at enqueue time, so sending it twice (tab closed while "sending", two tabs
 * flushing, a lost response) applies it once on the server. An operation
 * left "sending" by a closed tab is reset to "pending" at init. retry()
 * regenerates the key on purpose: the previous attempt was refused (4xx,
 * not applied), and the server would otherwise replay the stored refusal.
 *
 * Flushes are serialised in the tab and, where the Web Locks API exists,
 * across tabs; each flush re-reads the store, so operations queued by
 * another tab are sent too.
 */

export type QueuedStatus = 'pending' | 'sending' | 'conflict' | 'failed';

/** Fields managed by the queue; features provide the rest. */
export interface QueuedOperationBase {
  /** Local id (UUID). */
  id: string;
  /** Sent as Idempotency-Key; generated once at enqueue time. */
  idempotencyKey: string;
  /** ISO instant of the enqueue (display). */
  createdAt: string;
  /** Monotonic sequence number: the send order. */
  seq: number;
  status: QueuedStatus;
  /** Problem detail of the last failure (shown to the user). */
  error?: string;
}

type ManagedKey = 'id' | 'idempotencyKey' | 'createdAt' | 'seq' | 'status' | 'error';

/** Input of enqueue: the feature fields, optionally with a pre-generated key. */
export type NewQueuedOperation<Op extends QueuedOperationBase> = Op extends unknown
  ? Omit<Op, ManagedKey> & { idempotencyKey?: string }
  : never;

/** How a failed send is handled (see the lifecycle above). */
export type FailureOutcome = 'retry' | 'auth' | 'conflict' | 'failed';

export interface ClassifiedFailure {
  outcome: FailureOutcome;
  /** Text shown to the user (problem detail or title). */
  message?: string;
}

/** Default classification of the errors thrown by the API client (ApiProblem). */
export function classifyFailure(err: unknown): ClassifiedFailure {
  if (!(err instanceof ApiProblem)) {
    return { outcome: 'failed', message: err instanceof Error ? err.message : String(err) };
  }
  const message = err.detail || err.title;
  if (err.isNetworkError || err.status === 0) return { outcome: 'retry', message };
  if (err.status === 401) return { outcome: 'auth', message };
  if (err.status === 409 || err.status === 412 || err.status === 422) {
    return { outcome: 'conflict', message };
  }
  if (err.status === 429 || err.status >= 500) return { outcome: 'retry', message };
  return { outcome: 'failed', message };
}

export interface FlushResult {
  sent: number;
  conflicts: number;
  failed: number;
  /** True when the flush stopped on a transient error or a 401. */
  blocked: boolean;
}

export interface OperationQueueOptions<Op extends QueuedOperationBase> {
  store: KeyValueStore<Op>;
  /** Performs the request; resolves on 2xx, throws (ApiProblem) otherwise. */
  send: (op: Op) => Promise<void>;
  classify?: (err: unknown) => ClassifiedFailure;
  now?: () => Date;
  newId?: () => string;
  /** Called after an operation was applied by the server. */
  onSent?: (op: Op) => void;
  /** Called when an operation became `conflict` or `failed`. */
  onRejected?: (op: Op) => void;
  /** Web Locks name serialising flushes across tabs. */
  lockName?: string;
}

export interface OperationQueue<Op extends QueuedOperationBase> {
  /** Loads the persisted operations (idempotent; awaited by every method). */
  init(): Promise<void>;
  enqueue(input: NewQueuedOperation<Op>): Promise<Op>;
  /** Current operations in send order (stable reference between changes). */
  getSnapshot(): readonly Op[];
  subscribe(listener: () => void): () => void;
  /** Sends the pending operations in order; resolves when drained or blocked. */
  flush(): Promise<FlushResult>;
  /** Puts a conflict/failed operation back in the queue with a new key. */
  retry(id: string): Promise<void>;
  discard(id: string): Promise<void>;
  /** Clears every operation (logout). */
  clear(): Promise<void>;
  /** True after a 401, until resume(). */
  isPaused(): boolean;
  resume(): void;
  isFlushing(): boolean;
}

export function defaultNewId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `k-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

function bySeq<Op extends QueuedOperationBase>(a: Op, b: Op): number {
  return a.seq - b.seq;
}

export function createOperationQueue<Op extends QueuedOperationBase>(
  options: OperationQueueOptions<Op>,
): OperationQueue<Op> {
  const { store, send } = options;
  const classify = options.classify ?? classifyFailure;
  const now = options.now ?? (() => new Date());
  const newId = options.newId ?? defaultNewId;
  const listeners = new Set<() => void>();

  let ops: Op[] = [];
  let snapshot: readonly Op[] = ops;
  let nextSeq = 1;
  let paused = false;
  let loaded: Promise<void> | null = null;
  let flushing: Promise<FlushResult> | null = null;

  const emit = () => {
    snapshot = [...ops].sort(bySeq);
    for (const l of listeners) l();
  };

  const reload = async () => {
    const stored = await store.values();
    const reset: Op[] = [];
    for (const op of stored) {
      // A tab closed mid-request: replay it (the idempotency key makes it safe).
      if (op.status === 'sending') {
        const fixed = { ...op, status: 'pending' } as Op;
        await store.set(fixed.id, fixed);
        reset.push(fixed);
      } else reset.push(op);
    }
    ops = reset;
    nextSeq = Math.max(nextSeq, ...ops.map((o) => o.seq + 1), 1);
    emit();
  };

  const init = () => {
    loaded ??= reload().catch((err: unknown) => {
      loaded = null;
      throw err;
    });
    return loaded;
  };

  const put = async (op: Op) => {
    await store.set(op.id, op);
    ops = ops.some((o) => o.id === op.id)
      ? ops.map((o) => (o.id === op.id ? op : o))
      : [...ops, op];
    emit();
  };

  const remove = async (id: string) => {
    await store.delete(id);
    ops = ops.filter((o) => o.id !== id);
    emit();
  };

  const withLock = <T>(fn: () => Promise<T>): Promise<T> => {
    const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
    if (!options.lockName || !locks) return fn();
    return locks.request(options.lockName, fn) as Promise<T>;
  };

  const drain = async (): Promise<FlushResult> => {
    const result: FlushResult = { sent: 0, conflicts: 0, failed: 0, blocked: false };
    // Another tab may have sent or queued operations meanwhile.
    await reload();
    for (;;) {
      if (paused) {
        result.blocked = ops.some((o) => o.status === 'pending');
        break;
      }
      const next = ops.filter((o) => o.status === 'pending').sort(bySeq)[0];
      if (!next) break;
      const sending = { ...next, status: 'sending' } as Op;
      await put(sending);
      try {
        await send(sending);
        await remove(sending.id);
        result.sent++;
        options.onSent?.(sending);
      } catch (err) {
        const { outcome, message } = classify(err);
        if (outcome === 'retry' || outcome === 'auth') {
          await put({ ...sending, status: 'pending', error: message } as Op);
          if (outcome === 'auth') paused = true;
          result.blocked = true;
          break;
        }
        const rejected = {
          ...sending,
          status: outcome === 'conflict' ? 'conflict' : 'failed',
          error: message,
        } as Op;
        await put(rejected);
        if (outcome === 'conflict') result.conflicts++;
        else result.failed++;
        options.onRejected?.(rejected);
      }
    }
    return result;
  };

  return {
    init,
    async enqueue(input) {
      await init();
      const op = {
        ...input,
        id: newId(),
        idempotencyKey: input.idempotencyKey ?? newId(),
        createdAt: now().toISOString(),
        seq: nextSeq++,
        status: 'pending',
      } as unknown as Op;
      await put(op);
      return op;
    },
    getSnapshot: () => snapshot,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    flush() {
      flushing ??= init()
        .then(() => withLock(drain))
        .finally(() => {
          flushing = null;
          emit();
        });
      return flushing;
    },
    async retry(id) {
      await init();
      const op = ops.find((o) => o.id === id);
      if (!op || (op.status !== 'conflict' && op.status !== 'failed')) return;
      const again = { ...op, status: 'pending', idempotencyKey: newId(), seq: nextSeq++ } as Op;
      delete again.error;
      await put(again);
    },
    async discard(id) {
      await init();
      await remove(id);
    },
    async clear() {
      await store.clear();
      ops = [];
      paused = false;
      emit();
    },
    isPaused: () => paused,
    resume() {
      paused = false;
      emit();
    },
    isFlushing: () => flushing !== null,
  };
}
