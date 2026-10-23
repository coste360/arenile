# Beach map renderer

Shared SVG map of the admin panel (editor, operational map, spot picker) and of the public booking page.
Spec 11, ADR 0009 (shadcn only) and ADR 0012 (map kit).

## Layers

1. `kit/MapBackdrop` — sand, sea band and shoreline in the soft look of the kit (`--beach-*` tokens).
2. Decorations — `kit/DecorationArt`: one `<use>` per element; modular walkway tiles repeated at a fixed scale.
3. Spots — `kit/SpotArt`: status halo (first child, focus ring target) under the art, `StatusBadge` glyph in the
   corner, indicator dots (extra, note, VIP) in the other corner, `MapLabel` pill under the art.
4. `kit/MapKitDefs` — the ONE hidden `<svg><defs>` of the page with only the symbols in use (+ badge glyphs).

Booking status never goes inside the illustration: it is the halo (`STATUS_CLASSES`, `--status-*`) plus a badge
glyph for the states that need action (see `kit/status.ts`), so it reads in light, dark and greyscale.

## Performance

- Each element is a `<use href="#ark-<id>">`: 1,000 umbrellas share one symbol (test in `kit/kit.test.tsx`).
- Symbol bodies (~10 KB gzip) are a lazy chunk (`kit/symbols.ts`), outside the booking first screen.
- Spots and decorations are memoised; the editor drags by setting a `translate` on the dragged nodes only.

## Adding an illustration to the kit

1. Drop the SVG in `assets/beach-map-kit/svg/<id>.svg`: viewBox `0 0 120 120`, top-down, colours as
   `var(--beach-*, #fallback)`, only plain shapes (the generator rejects scripts, styles, `<use>`, images,
   links, event attributes and external references).
2. Add its entry to `assets/beach-map-kit/manifest.json` (`id`, Italian `label`, `category`, `type`, `file`).
   A new bookable family also needs a `SPOT_KIND_RULES` entry in `web/scripts/gen-map-kit.mjs`.
3. Add the label under `kit.asset.<id>` in `web/admin/src/features/map-editor/locales/{it,en,de,fr}.json`
   (Italian = manifest label; a test checks it).
4. Run `make gen` (writes `kit/generated/*` and `internal/domain/mapping/kitassets_gen.go`), then `make check`.

Never edit the generated files nor the original kit files to fix the app: put adjustments in the generator
(`NORMALISE`, `VARIANTS`) and document them in ADR 0012.
