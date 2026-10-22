---
paths:
  - "web/**/*.{ts,tsx}"
---
- Server data only through TanStack Query hooks built on the generated client.
- Forms: react-hook-form + zod; schemas live next to the form.
- UI primitives from `web/shared/ui` (shadcn/ui); no new component libraries.
- `web/booking` has a 150 KB gzip JS budget: no heavy dependencies, lazy-load the map.
- Every interactive map element needs a role, an aria-label and keyboard support.
- All text through i18next keys; add the Italian string and an English one.
- API types come from `web/shared/api/generated/schema.d.ts` (openapi-typescript) used with
  openapi-fetch; never edit `web/shared/api/generated/**`, run `make gen` instead.
- Amounts are integer cents; format them only at display time with `Intl.NumberFormat`.
