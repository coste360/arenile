import { describe, expect, it, vi } from 'vitest';
import { ApiProblem } from '../api/client';
import { createMemoryStore } from './kv';
import {
  classifyFailure,
  createOperationQueue,
  type OperationQueueOptions,
  type QueuedOperationBase,
} from './queue';

interface TestOp extends QueuedOperationBase {
  kind: 'a' | 'b';
  payload: string;
}

function problem(status: number, detail = `HTTP ${status}`) {
  return new ApiProblem({ type: 'conflict', status, title: 'T', detail });
}
const networkError = () => new ApiProblem({ type: 'network_error', status: 0, title: 'Network' });

function setup(
  send: (op: TestOp) => Promise<void>,
  backing = new Map<string, TestOp>(),
  extra: Partial<OperationQueueOptions<TestOp>> = {},
) {
  let n = 0;
  const queue = createOperationQueue<TestOp>({
    store: createMemoryStore(backing),
    send,
    newId: () => `id-${++n}`,
    now: () => new Date('2026-07-14T08:00:00Z'),
    ...extra,
  });
  return { queue, backing };
}

describe('classifyFailure', () => {
  it.each([
    [networkError(), 'retry'],
    [problem(401), 'auth'],
    [problem(409), 'conflict'],
    [problem(412), 'conflict'],
    [problem(422), 'conflict'],
    [problem(429), 'retry'],
    [problem(503), 'retry'],
    [problem(403), 'failed'],
    [problem(404), 'failed'],
    [new Error('boom'), 'failed'],
  ])('%s → %s', (err, outcome) => {
    expect(classifyFailure(err).outcome).toBe(outcome);
  });

  it('uses the problem detail as message', () => {
    expect(classifyFailure(problem(409, 'Cassa chiusa')).message).toBe('Cassa chiusa');
  });
});

describe('OperationQueue_Enqueue', () => {
  it('assigns id, idempotency key, sequence and pending status, and persists', async () => {
    const { queue, backing } = setup(async () => {});
    const op = await queue.enqueue({ kind: 'a', payload: 'x' });
    expect(op).toMatchObject({
      id: 'id-1',
      idempotencyKey: 'id-2',
      seq: 1,
      status: 'pending',
      createdAt: '2026-07-14T08:00:00.000Z',
    });
    expect(backing.get('id-1')).toEqual(op);
    expect(queue.getSnapshot()).toEqual([op]);
  });

  it('keeps a key provided by the caller (request already attempted online)', async () => {
    const { queue } = setup(async () => {});
    const op = await queue.enqueue({ kind: 'a', payload: 'x', idempotencyKey: 'given' });
    expect(op.idempotencyKey).toBe('given');
  });

  it('survives a reload: a new queue over the same storage sees the operations', async () => {
    const backing = new Map<string, TestOp>();
    const first = setup(async () => {}, backing).queue;
    await first.enqueue({ kind: 'a', payload: '1' });
    await first.enqueue({ kind: 'b', payload: '2' });

    const second = setup(async () => {}, backing).queue;
    await second.init();
    expect(second.getSnapshot().map((o) => o.payload)).toEqual(['1', '2']);
    // Sequence continues after the persisted ones.
    const third = await second.enqueue({ kind: 'a', payload: '3' });
    expect(third.seq).toBe(3);
  });

  it('notifies subscribers with a new snapshot reference', async () => {
    const { queue } = setup(async () => {});
    const listener = vi.fn();
    const unsubscribe = queue.subscribe(listener);
    const before = queue.getSnapshot();
    await queue.enqueue({ kind: 'a', payload: 'x' });
    expect(listener).toHaveBeenCalled();
    expect(queue.getSnapshot()).not.toBe(before);
    unsubscribe();
  });
});

describe('OperationQueue_Flush', () => {
  it('sends in enqueue order with the stored idempotency key and empties the queue', async () => {
    const sent: string[] = [];
    const { queue, backing } = setup(async (op) => {
      sent.push(`${op.payload}:${op.idempotencyKey}`);
    });
    const a = await queue.enqueue({ kind: 'a', payload: '1' });
    const b = await queue.enqueue({ kind: 'b', payload: '2' });
    const res = await queue.flush();
    expect(sent).toEqual([`1:${a.idempotencyKey}`, `2:${b.idempotencyKey}`]);
    expect(res).toEqual({ sent: 2, conflicts: 0, failed: 0, blocked: false });
    expect(queue.getSnapshot()).toEqual([]);
    expect(backing.size).toBe(0);
  });

  it('stops on a network error keeping the order, and resumes from the same operation', async () => {
    let online = false;
    const sent: string[] = [];
    const { queue } = setup(async (op) => {
      if (!online) throw networkError();
      sent.push(op.payload);
    });
    await queue.enqueue({ kind: 'a', payload: '1' });
    await queue.enqueue({ kind: 'a', payload: '2' });
    const first = await queue.flush();
    expect(first.blocked).toBe(true);
    expect(queue.getSnapshot().map((o) => o.status)).toEqual(['pending', 'pending']);

    online = true;
    await queue.flush();
    expect(sent).toEqual(['1', '2']);
  });

  it('marks 409/412/422 as conflict with the detail and goes on with the others', async () => {
    const onRejected = vi.fn();
    const { queue } = setup(
      async (op) => {
        if (op.payload === 'bad') throw problem(409, 'Sessione di cassa chiusa');
      },
      undefined,
      { onRejected },
    );
    await queue.enqueue({ kind: 'a', payload: 'bad' });
    await queue.enqueue({ kind: 'a', payload: 'good' });
    const res = await queue.flush();
    expect(res).toMatchObject({ sent: 1, conflicts: 1, blocked: false });
    expect(queue.getSnapshot()).toEqual([
      expect.objectContaining({
        payload: 'bad',
        status: 'conflict',
        error: 'Sessione di cassa chiusa',
      }),
    ]);
    expect(onRejected).toHaveBeenCalledWith(expect.objectContaining({ payload: 'bad' }));
  });

  it('marks other 4xx as failed', async () => {
    const { queue } = setup(async () => {
      throw problem(403, 'Permesso negato');
    });
    await queue.enqueue({ kind: 'a', payload: 'x' });
    const res = await queue.flush();
    expect(res.failed).toBe(1);
    expect(queue.getSnapshot()[0]?.status).toBe('failed');
  });

  it('pauses on 401 until resume()', async () => {
    let authed = false;
    const send = vi.fn(async () => {
      if (!authed) throw problem(401);
    });
    const { queue } = setup(send);
    await queue.enqueue({ kind: 'a', payload: '1' });
    await queue.enqueue({ kind: 'a', payload: '2' });
    await queue.flush();
    expect(queue.isPaused()).toBe(true);
    expect(send).toHaveBeenCalledTimes(1);

    authed = true;
    expect((await queue.flush()).blocked).toBe(true);
    expect(send).toHaveBeenCalledTimes(1);

    queue.resume();
    await queue.flush();
    expect(queue.getSnapshot()).toEqual([]);
  });

  it('serialises concurrent flushes (each operation is sent once)', async () => {
    const send = vi.fn(async () => {
      await new Promise((r) => setTimeout(r, 5));
    });
    const { queue } = setup(send);
    await queue.enqueue({ kind: 'a', payload: '1' });
    await Promise.all([queue.flush(), queue.flush(), queue.flush()]);
    expect(send).toHaveBeenCalledTimes(1);
  });

  it('replays with the same key an operation left "sending" by a closed tab', async () => {
    const backing = new Map<string, TestOp>();
    const crashed = setup(() => new Promise<void>(() => {}), backing).queue;
    const op = await crashed.enqueue({ kind: 'a', payload: '1' });
    void crashed.flush(); // never settles: the tab is "closed" mid-request
    await vi.waitFor(() => expect(backing.get(op.id)?.status).toBe('sending'));

    const keys: string[] = [];
    const reopened = setup(async (o) => {
      keys.push(o.idempotencyKey);
    }, backing).queue;
    await reopened.init();
    expect(reopened.getSnapshot()[0]?.status).toBe('pending');
    await reopened.flush();
    expect(keys).toEqual([op.idempotencyKey]);
  });
});

describe('OperationQueue_RetryDiscard', () => {
  it('retry puts a conflict back as pending with a new idempotency key', async () => {
    let fail = true;
    const { queue } = setup(async () => {
      if (fail) throw problem(422);
    });
    const op = await queue.enqueue({ kind: 'a', payload: 'x' });
    await queue.flush();
    await queue.retry(op.id);
    const again = queue.getSnapshot()[0];
    expect(again?.status).toBe('pending');
    expect(again?.error).toBeUndefined();
    expect(again?.idempotencyKey).not.toBe(op.idempotencyKey);
    fail = false;
    await queue.flush();
    expect(queue.getSnapshot()).toEqual([]);
  });

  it('retry ignores pending operations', async () => {
    const { queue } = setup(async () => {});
    const op = await queue.enqueue({ kind: 'a', payload: 'x' });
    await queue.retry(op.id);
    expect(queue.getSnapshot()[0]?.idempotencyKey).toBe(op.idempotencyKey);
  });

  it('discard and clear remove from memory and storage', async () => {
    const { queue, backing } = setup(async () => {});
    const a = await queue.enqueue({ kind: 'a', payload: '1' });
    await queue.enqueue({ kind: 'a', payload: '2' });
    await queue.discard(a.id);
    expect(backing.has(a.id)).toBe(false);
    await queue.clear();
    expect(backing.size).toBe(0);
    expect(queue.getSnapshot()).toEqual([]);
  });
});
