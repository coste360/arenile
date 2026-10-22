import { describe, expect, it, vi } from 'vitest';
import { ApiProblem, call, createApiClient, readCookie, toApiProblem } from './client';

function jsonResponse(status: number, body: unknown, type = 'application/json') {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': type } });
}

describe('readCookie', () => {
  it.each([
    ['arenile_csrf=abc; other=1', 'abc'],
    ['other=1;  arenile_csrf=a%20b', 'a b'],
    ['other=1', undefined],
    ['', undefined],
  ])('%j', (cookies, expected) => {
    expect(readCookie('arenile_csrf', cookies)).toBe(expected);
  });
});

describe('createApiClient', () => {
  it('adds X-CSRF-Token on unsafe methods only', async () => {
    const fetchMock = vi.fn(async (req: Request) => {
      void req;
      return jsonResponse(200, {
        principal_type: 'user',
        memberships: [],
        totp_enrolment_required: false,
      });
    });
    const client = createApiClient({
      fetch: fetchMock,
      getCsrfToken: () => 'tok',
      baseUrl: 'http://x',
    });
    await client.GET('/api/admin/v1/auth/me');
    await client.POST('/api/admin/v1/auth/login', { body: { email: 'a@b.it', password: 'p' } });
    const [getReq, postReq] = fetchMock.mock.calls.map((c) => c[0]);
    expect(getReq?.headers.get('X-CSRF-Token')).toBeNull();
    expect(postReq?.headers.get('X-CSRF-Token')).toBe('tok');
    expect(postReq?.credentials).toBe('same-origin');
  });

  it('turns problem+json into ApiProblem', async () => {
    const client = createApiClient({
      baseUrl: 'http://x',
      fetch: async () =>
        jsonResponse(
          401,
          { type: 'invalid_credentials', title: 'Credenziali non valide', status: 401 },
          'application/problem+json',
        ),
    });
    const err = await call(() =>
      client.POST('/api/admin/v1/auth/login', { body: { email: 'a@b.it', password: 'x' } }),
    ).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiProblem);
    expect((err as ApiProblem).type).toBe('invalid_credentials');
    expect((err as ApiProblem).status).toBe(401);
  });

  it('maps fetch failures to network problems', async () => {
    const client = createApiClient({
      baseUrl: 'http://x',
      fetch: async () => {
        throw new TypeError('Failed to fetch');
      },
    });
    const err = (await call(() => client.GET('/api/admin/v1/auth/me')).catch(
      (e: unknown) => e,
    )) as ApiProblem;
    expect(err.isNetworkError).toBe(true);
  });
});

describe('toApiProblem', () => {
  it('builds a problem from a non-problem response', () => {
    const p = toApiProblem(
      'oops',
      new Response('x', { status: 429, headers: { 'Retry-After': '30' } }),
    );
    expect(p.type).toBe('rate_limited');
    expect(p.retryAfter).toBe(30);
  });
});
