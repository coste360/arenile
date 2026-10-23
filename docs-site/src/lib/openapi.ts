/**
 * Build-time view of the API contract (api/openapi.yaml) for <ApiOperation>:
 * the same file the server code and the generated reference come from.
 */
import yaml from 'js-yaml';
// Raw text through Vite: works from the bundled prerender chunks and is
// watched by `astro dev`.
import contract from '../../../api/openapi.yaml?raw';
import { API_REFERENCE_BASE } from './constants.mjs';
const METHODS = ['get', 'put', 'post', 'delete', 'patch'] as const;

export interface ApiOperationInfo {
  id: string;
  method: (typeof METHODS)[number];
  path: string;
  summary: string;
  tags: string[];
  /** True when the operation needs no credentials (public booking API, webhooks). */
  public: boolean;
  deprecated: boolean;
  /** Path of the generated reference page (without the site base). */
  href: string;
}

interface RawOperation {
  operationId?: string;
  summary?: string;
  tags?: string[];
  security?: unknown[];
  deprecated?: boolean;
}

let cache: Map<string, ApiOperationInfo> | undefined;

/** starlight-openapi slug of an operation id (github-slugger on [A-Za-z0-9_] ids). */
const slug = (id: string) => id.toLowerCase().replace(/[^a-z0-9_-]/g, '');

export function apiOperations(): Map<string, ApiOperationInfo> {
  if (cache) return cache;
  const doc = yaml.load(contract) as {
    security?: unknown[];
    paths: Record<string, Partial<Record<(typeof METHODS)[number], RawOperation>>>;
  };
  const ids = new Map<string, number>();
  for (const item of Object.values(doc.paths)) {
    for (const m of METHODS) if (item[m]?.operationId) ids.set(item[m]!.operationId!, (ids.get(item[m]!.operationId!) ?? 0) + 1);
  }
  cache = new Map();
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const method of METHODS) {
      const op = item[method];
      if (!op?.operationId) continue;
      const security = op.security ?? doc.security ?? [];
      const dup = (ids.get(op.operationId) ?? 0) > 1;
      cache.set(op.operationId, {
        id: op.operationId,
        method,
        path,
        summary: op.summary ?? op.operationId,
        tags: op.tags ?? [],
        public: security.length === 0,
        deprecated: !!op.deprecated,
        href: `/${API_REFERENCE_BASE}/operations/${slug(op.operationId)}${dup ? `/${method}` : ''}/`,
      });
    }
  }
  return cache;
}

/** Looks an operation up by operationId; unknown ids fail the build. */
export function apiOperation(id: string): ApiOperationInfo {
  const op = apiOperations().get(id);
  if (!op) throw new Error(`<ApiOperation id="${id}">: unknown operationId in api/openapi.yaml`);
  return op;
}
