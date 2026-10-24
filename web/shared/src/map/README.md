# Beach map renderer

Shared SVG map of the admin panel (editor, operational map, spot picker) and of the public booking page.
Spec 11, ADR 0009 (shadcn only), ADR 0012 (map kit) and ADR 0024 (v3 kit and kit choice).

## Kits

Two kits ship in the binary (`assets/map-kits/<key>`): `modern` (v3, default) and `classic` (v2.1), with the
same asset ids. The establishment chooses one (`theme.map_kit`); `MapKitProvider` hands it to every map below
it (the admin `/e/$id` layout; the booking page passes `kit` to `BeachMap`), and `BeachMap kit` overrides it
for previews (`MapKitPreview`). `useMapKit()` reads it; unknown values fall back to `DEFAULT_MAP_KIT`.

## Layers

1. `kit/MapBackdrop` — sand, sea band and shoreline in the soft look of the kit (`--beach-*` tokens).
2. Decorations — `kit/DecorationArt`: one `<use>` per element; modular walkway tiles repeated at a fixed scale.
3. Spots — `kit/SpotArt`, then the `StatusBadge` glyph in the top-right corner, indicator dots (extra, note,
   VIP) in the other corner, `MapLabel` pill under the art. The first child is the focus/hover target:
   - classic (`halo` style): a status halo under the art (`STATUS_CLASSES`, `--status-*`);
   - modern (`fill` style): an outline ring, invisible at rest; the art paints its status part with
     `--beach-state`, set on its group by `STATUS_CLASSES[status].state`. The badge sits in the corner the kit
     keeps empty (`badgeOffset`). A literal colour override falls back to the halo.
   Blocked spots get a diagonal bar (`BlockedBar`) in both kits.
4. `kit/MapKitDefs` — the ONE hidden `<svg><defs>` of the page with only the symbols in use, per kit
   (+ badge glyphs).

Status always reads without colour: a badge glyph for every booked state (see `kit/status.ts`) and the bar on
blocked spots, so the map reads in light, dark and greyscale. The `--status-*` tokens have light and dark values
(`DEFAULT_STATUS_COLORS`, `DARK_STATUS_COLORS`); `theme/brand.test.ts` checks them on the sand.

## Performance

- Each element is a `<use href="#ark-<kit>-<id>">`: 1,000 umbrellas share one symbol (test in
  `kit/kit.test.tsx`).
- Symbol bodies (~10 KB gzip per kit) are a lazy chunk per kit (`kit/symbols.ts`), outside the booking first
  screen; a page loads only the kits it draws.
- Spots and decorations are memoised; the editor drags by setting a `translate` on the dragged nodes only.

## Adding an illustration

Every kit must have the same catalogue, so a new asset goes into **every** kit:

1. Drop the SVG in `assets/map-kits/<key>/svg/<id>.svg` for each kit: viewBox `0 0 120 120`, top-down, colours
   as `var(--beach-*, #fallback)`, only plain shapes (the generator rejects scripts, styles, `<use>`, images,
   links, event attributes and external references). The modern kit (`fill` profile) also requires: elements
   `g/path/rect/circle/ellipse` only, the corner x 92–120, y 0–28 empty, and bookable assets painting their
   status with `var(--beach-state, …)`.
2. Add its entry to each `manifest.json` (`id`, Italian `label`, `category`, `type`, `file`; modern also
   `stateful`, `cornerFree`). A new bookable family also needs a `SPOT_KIND_RULES` entry in
   `web/scripts/gen-map-kit.mjs`.
3. Add the label under `kit.asset.<id>` in `web/admin/src/features/map-editor/locales/{it,en,de,fr}.json`
   (Italian = manifest label of the default kit; a test checks it).
4. Run `make gen` (writes `kit/generated/*` and `internal/domain/mapping/kitassets_gen.go`), then `make check`.

Never edit the generated files nor the original kit files to fix the app: put adjustments in the generator
(`NORMALISE` per kit, `VARIANTS`) and document them in ADR 0012 / 0024.
