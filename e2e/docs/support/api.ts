import type { APIRequestContext } from "@playwright/test";
import type { components } from "../../../web/shared/api/generated/schema";
import { DEMO_OWNER_EMAIL, DOCS_OWNER_PASSWORD } from "./env";

/** API schemas of the contract (types only, from api/openapi.yaml via make gen). */
export type Schemas = components["schemas"];

const ADMIN = "/api/admin/v1";

/** Thin client of the admin API for the capture run (cookie session + CSRF header). */
export class AdminApi {
  private constructor(
    readonly request: APIRequestContext,
    private csrf: string,
  ) {}

  /** Logs in as the demo owner; the request context keeps the session cookies. */
  static async login(request: APIRequestContext): Promise<AdminApi> {
    const res = await request.post(`${ADMIN}/auth/login`, {
      data: { email: DEMO_OWNER_EMAIL, password: DOCS_OWNER_PASSWORD },
    });
    if (!res.ok()) throw new Error(`login -> ${res.status()} ${await res.text()}`);
    const state = await request.storageState();
    const csrf = state.cookies.find((c) => c.name === "arenile_csrf")?.value ?? "";
    return new AdminApi(request, csrf);
  }

  async get<T>(path: string, query?: Record<string, string | number | boolean | string[]>): Promise<T> {
    const url = new URL(`${ADMIN}${path}`, "http://x");
    for (const [k, v] of Object.entries(query ?? {})) {
      for (const item of Array.isArray(v) ? v : [v]) url.searchParams.append(k, String(item));
    }
    const res = await this.request.get(url.pathname + url.search);
    if (!res.ok()) throw new ApiError("GET", path, res.status(), await res.text());
    return (await res.json()) as T;
  }

  async post<T>(path: string, data: unknown): Promise<T> {
    const res = await this.request.post(`${ADMIN}${path}`, {
      data,
      headers: { "X-CSRF-Token": this.csrf },
    });
    if (!res.ok()) throw new ApiError("POST", path, res.status(), await res.text());
    return (await res.json()) as T;
  }

  /** Every page of a cursor-paginated collection. */
  async all<T>(path: string, query: Record<string, string | number> = {}): Promise<T[]> {
    const out: T[] = [];
    let cursor: string | undefined;
    do {
      const page = await this.get<{ items: T[]; next_cursor?: string | null }>(path, {
        ...query,
        limit: 200,
        ...(cursor ? { cursor } : {}),
      });
      out.push(...page.items);
      cursor = page.next_cursor ?? undefined;
    } while (cursor);
    return out;
  }
}

export class ApiError extends Error {
  constructor(
    readonly method: string,
    readonly path: string,
    readonly status: number,
    body: string,
  ) {
    super(`${method} ${path} -> ${status}: ${body.slice(0, 300)}`);
  }
}
