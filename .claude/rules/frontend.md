---
paths:
  - "web/**/*.{ts,tsx}"
---
- Server data only through TanStack Query hooks built on the generated client.
- Forms: react-hook-form + zod; schemas live next to the form.
- UI = shadcn/ui only (ADR 0009), imported from `@arenile/shared/ui` (booking: per-file
  `@arenile/shared/ui/<name>`) and `@arenile/shared/ui/patterns`; no other component library.
  Missing component: `cd web && npx shadcn@latest add <name> --cwd shared`, then export it from
  `shared/src/ui/index.ts` (see `web/shared/src/ui/README.md`). Never hand-roll what shadcn has.
- No custom CSS: no new `.css` files, CSS modules, CSS-in-JS or `style={{…}}` (only dynamic SVG
  geometry in `shared/src/map` and the map editor). Style with component variants and Tailwind
  utilities on theme tokens (`bg-primary`, `text-muted-foreground`), never raw colours or hex.
  The only stylesheet is `web/shared/src/styles/globals.css` (theme variables). The CSS guard in
  `npm run lint` fails otherwise and has no exception lists.
- Dates via `DatePicker`/`DateRangePicker` and `formatIsoDate` (dd/MM/yyyy, never
  `<input type="date">`); money via `MoneyInput` and `formatCents` (Intl it-IT).
- Light and dark mode: check both when changing a screen.
- `web/booking` has a 150 KB gzip JS budget: no heavy dependencies, lazy-load the map.
- Every interactive map element needs a role, an aria-label and keyboard support.
- All text through i18next keys; add the Italian string and an English one.
- API types come from `web/shared/api/generated/schema.d.ts` (openapi-typescript) used with
  openapi-fetch; never edit `web/shared/api/generated/**`, run `make gen` instead.
- Amounts are integer cents; format them only at display time with `Intl.NumberFormat`.
