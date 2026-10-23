import { ApiProblem } from '@arenile/shared/api';
import { describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { BookingProblem, callBooking, describeProblem, groupConflicts } from './problem';

const t = createAdminTestI18n().getFixedT('it', 'bookings');

function problem(type: string, extra: Record<string, unknown> = {}, status = 409) {
  const raw = { type, title: type, status, ...extra };
  return new BookingProblem(new ApiProblem(raw as never), raw);
}

describe('groupConflicts', () => {
  it('groups by resource, merges AM+PM into FULL and sorts by date', () => {
    const groups = groupConflicts([
      { spot_id: 's1', date: '2026-07-15', slot: 'PM', reason: 'spot_unavailable' },
      { spot_id: 's1', date: '2026-07-14', slot: 'AM', reason: 'spot_unavailable' },
      { spot_id: 's1', date: '2026-07-14', slot: 'PM', reason: 'spot_unavailable' },
      { pool_id: 'p1', date: '2026-07-14', slot: 'AM', reason: 'pool_exhausted' },
    ]);
    expect(groups).toEqual([
      {
        resourceId: 's1',
        kind: 'spot',
        reason: 'spot_unavailable',
        days: [
          { date: '2026-07-14', slots: 'FULL' },
          { date: '2026-07-15', slots: 'PM' },
        ],
      },
      {
        resourceId: 'p1',
        kind: 'pool',
        reason: 'pool_exhausted',
        days: [{ date: '2026-07-14', slots: 'AM' }],
      },
    ]);
  });
});

describe('describeProblem', () => {
  it('lists the conflicting dates of spot_unavailable with the spot label', () => {
    const p = problem('spot_unavailable', {
      conflicts: [
        { spot_id: 's1', date: '2026-07-14', slot: 'AM', reason: 'spot_unavailable' },
        { spot_id: 's1', date: '2026-07-14', slot: 'PM', reason: 'spot_unavailable' },
        { spot_id: 's1', date: '2026-07-15', slot: 'PM', reason: 'spot_unavailable' },
      ],
    });
    const text = describeProblem(p, t, {
      resourceLabel: (g) => (g.resourceId === 's1' ? 'A12' : undefined),
    });
    expect(text.title).toBe('Una o più postazioni non sono disponibili per le date scelte.');
    expect(text.details).toEqual(['A12: 14/07/2026, 15/07/2026 (Pomeriggio)']);
    expect(text.reload).toBe(false);
  });

  it('truncates long conflict lists', () => {
    const conflicts = Array.from({ length: 9 }, (_, i) => ({
      spot_id: 's1',
      date: `2026-07-${String(10 + i)}`,
      slot: 'AM',
      reason: 'spot_unavailable',
    }));
    const text = describeProblem(problem('spot_unavailable', { conflicts }), t);
    expect(text.details[0]).toMatch(/^Postazione: 10\/07\/2026 \(Mattina\).*e altri 3 giorni$/);
  });

  it('maps pool_exhausted and quota_exceeded', () => {
    expect(describeProblem(problem('pool_exhausted'), t).title).toMatch(/Quantità non disponibile/);
    expect(describeProblem(problem('quota_exceeded'), t).title).toMatch(/quota online/);
  });

  it('names the current status of invalid_transition and offers a reload', () => {
    const text = describeProblem(problem('invalid_transition', { current_status: 'cancelled' }), t);
    expect(text.title).toBe('Operazione non ammessa: ora la prenotazione è «Cancellata».');
    expect(text.reload).toBe(true);
  });

  it('treats version_conflict and a bare 412 as stale data', () => {
    expect(describeProblem(problem('version_conflict', {}, 412), t).reload).toBe(true);
    expect(
      describeProblem(new ApiProblem({ type: 'unknown_error', status: 412, title: 'x' }), t).reload,
    ).toBe(true);
  });

  it('translates policy violations with their parameters', () => {
    const text = describeProblem(
      problem('policy_violation', { violations: [{ code: 'too_many_spots' }] }, 422),
      t,
    );
    expect(text.details).toEqual(['Troppe postazioni in una prenotazione.']);
  });

  it('explains cash_session_closed and validation errors', () => {
    expect(describeProblem(problem('cash_session_closed'), t).title).toMatch(/cassa è chiusa/);
    const v = describeProblem(
      new ApiProblem({
        type: 'validation_failed',
        status: 400,
        title: 'x',
        errors: [{ field: 'amount', message: 'too big' }],
      }),
      t,
    );
    expect(v.details).toEqual(['amount: too big']);
  });

  it('falls back to the generic message for unknown errors', () => {
    expect(describeProblem(new Error('boom'), t).title).toMatch(/errore/);
  });
});

describe('callBooking', () => {
  it('keeps conflicts and current_status from the problem body', async () => {
    const body = {
      type: 'spot_unavailable',
      title: 'x',
      status: 409,
      conflicts: [
        { spot_id: 's1', date: '2026-07-14', slot: 'AM', reason: 'spot_unavailable' },
        { bogus: true },
      ],
    };
    const response = new Response(JSON.stringify(body), { status: 409 });
    const err = await callBooking(async () => ({ error: body, response })).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(BookingProblem);
    expect((err as BookingProblem).type).toBe('spot_unavailable');
    expect((err as BookingProblem).conflicts).toHaveLength(1);
  });

  it('maps thrown fetch failures to network problems', async () => {
    const err = await callBooking(async () => {
      throw new TypeError('offline');
    }).catch((e: unknown) => e);
    expect((err as BookingProblem).isNetworkError).toBe(true);
  });

  it('returns the data on success', async () => {
    await expect(
      callBooking(async () => ({ data: { ok: 1 }, response: new Response('{}') })),
    ).resolves.toEqual({ ok: 1 });
  });
});
