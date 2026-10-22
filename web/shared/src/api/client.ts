import createClient, { type Client, type Middleware } from 'openapi-fetch';
import type { components, paths } from '../../api/generated/schema';

export type Schemas = components['schemas'];
export type Problem = Schemas['Problem'];
export type ProblemType = Schemas['ProblemType'];
export type Me = Schemas['Me'];
export type Establishment = Schemas['Establishment'];
export type EstablishmentList = Schemas['EstablishmentList'];
export type PublicEstablishment = Schemas['PublicEstablishment'];
export type LoginRequest = Schemas['LoginRequest'];
export type ApiTheme = Schemas['Theme'];

export type ApiClient = Client<paths>;

/** Name of the double-submit CSRF cookie set at login (spec 16.2, openapi CSRFToken). */
export const CSRF_COOKIE = 'arenile_csrf';
export const CSRF_HEADER = 'X-CSRF-Token';

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);

/** Reads a cookie value from a `document.cookie`-style string. */
export function readCookie(name: string, cookies: string): string | undefined {
  for (const part of cookies.split(';')) {
    const eq = part.indexOf('=');
    if (eq < 0) continue;
    if (part.slice(0, eq).trim() === name) {
      const raw = part.slice(eq + 1).trim();
      try {
        return decodeURIComponent(raw);
      } catch {
        return raw;
      }
    }
  }
  return undefined;
}

/** Client-side error kinds that have no server problem type. */
export type ClientErrorType = 'network_error' | 'unknown_error';

/**
 * Error thrown by the hooks: either a server `application/problem+json`
 * (RFC 9457) or a client-side failure such as a network error.
 */
export class ApiProblem extends Error {
  readonly type: ProblemType | ClientErrorType;
  readonly status: number;
  readonly title: string;
  readonly detail?: string;
  readonly retryAfter?: number;
  readonly fieldErrors: Schemas['FieldError'][];
  readonly requestId?: string;

  constructor(problem: {
    type: ProblemType | ClientErrorType;
    status: number;
    title: string;
    detail?: string;
    retry_after?: number;
    errors?: Schemas['FieldError'][];
    request_id?: string;
  }) {
    super(problem.detail ?? problem.title);
    this.name = 'ApiProblem';
    this.type = problem.type;
    this.status = problem.status;
    this.title = problem.title;
    this.detail = problem.detail;
    this.retryAfter = problem.retry_after;
    this.fieldErrors = problem.errors ?? [];
    this.requestId = problem.request_id;
  }

  get isNetworkError(): boolean {
    return this.type === 'network_error';
  }
}

function isProblem(value: unknown): value is Problem {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as Problem).type === 'string' &&
    typeof (value as Problem).status === 'number'
  );
}

/** Converts an openapi-fetch error body and response into an ApiProblem. */
export function toApiProblem(error: unknown, response: Response | undefined): ApiProblem {
  if (error instanceof ApiProblem) return error;
  if (isProblem(error)) return new ApiProblem(error);
  if (!response) {
    return new ApiProblem({ type: 'network_error', status: 0, title: 'Network error' });
  }
  const retry = Number(response.headers.get('Retry-After'));
  return new ApiProblem({
    type: response.status === 429 ? 'rate_limited' : 'unknown_error',
    status: response.status,
    title: response.statusText || `HTTP ${response.status}`,
    retry_after: Number.isFinite(retry) && retry > 0 ? retry : undefined,
  });
}

/** Returns `data` or throws an ApiProblem. */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.error !== undefined || !result.response.ok) {
    throw toApiProblem(result.error, result.response);
  }
  return result.data as T;
}

/** Runs an openapi-fetch call, mapping thrown fetch failures to network problems. */
export async function call<T>(
  fn: () => Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  let result;
  try {
    result = await fn();
  } catch (err) {
    if (err instanceof ApiProblem) throw err;
    throw new ApiProblem({
      type: 'network_error',
      status: 0,
      title: 'Network error',
      detail: err instanceof Error ? err.message : undefined,
    });
  }
  return unwrap(result);
}

export interface ApiClientOptions {
  /** Origin or prefix of the API; empty means same origin. */
  baseUrl?: string;
  /** Custom fetch, mainly for tests. */
  fetch?: (request: Request) => Promise<Response>;
  /** Returns the CSRF token; defaults to reading the `arenile_csrf` cookie. */
  getCsrfToken?: () => string | undefined;
  /** Extra headers for every request (e.g. a booking session token). */
  headers?: () => Record<string, string | undefined>;
}

/** Middleware adding the double-submit CSRF header on unsafe methods. */
export function csrfMiddleware(getToken: () => string | undefined): Middleware {
  return {
    onRequest({ request }) {
      if (SAFE_METHODS.has(request.method.toUpperCase())) return undefined;
      const token = getToken();
      if (token && !request.headers.has(CSRF_HEADER)) request.headers.set(CSRF_HEADER, token);
      return request;
    },
  };
}

/** Creates the typed API client with cookie session and CSRF support. */
export function createApiClient(options: ApiClientOptions = {}): ApiClient {
  const client = createClient<paths>({
    baseUrl: options.baseUrl ?? '',
    credentials: 'same-origin',
    // Resolve fetch lazily so test doubles installed later are honoured.
    fetch: (input: Request) => (options.fetch ?? globalThis.fetch)(input),
    headers: { Accept: 'application/json, application/problem+json' },
  });
  const getCsrf =
    options.getCsrfToken ??
    (() =>
      typeof document === 'undefined' ? undefined : readCookie(CSRF_COOKIE, document.cookie));
  client.use(csrfMiddleware(getCsrf));
  if (options.headers) {
    const extra = options.headers;
    client.use({
      onRequest({ request }) {
        for (const [k, v] of Object.entries(extra())) if (v) request.headers.set(k, v);
        return request;
      },
    });
  }
  return client;
}
