# Beach map kit — provenance

© Coste360 SRL, original artwork created by Coste360 with the help of an image-generation model;
distributed with Arenile under the repository licence (BSL 1.1).

- Owner: Coste360 SRL. The artwork is original and not derived from third-party icon sets.
- Licence: the same as the repository (Business Source License 1.1, SPDX `BUSL-1.1`, converting to
  AGPL-3.0-or-later as described in `LICENSE` and `docs/adr/0007-licenza-bsl.md`).
- Source of truth: the files in this directory (`svg/*.svg`, `manifest.json`, `beach-tokens.css`,
  `catalogo.html`, `README.md`) are kept exactly as supplied. Never edit them to fix the app: the
  generator `web/scripts/gen-map-kit.mjs` (run by `make gen`) reads them, sanitises them and writes
  `web/shared/src/map/kit/generated/` and `internal/domain/mapping/kitassets_gen.go`. Adjustments
  the app needs (for example the axis-aligned `ombrellone_quadrato`, or the `asset` manifest type
  folded into `bookable`/`structure`) live in the generator and are documented in
  `docs/adr/0012-kit-mappa.md`.
- To add an asset: drop the SVG in `svg/`, add its entry to `manifest.json`, add its label to the
  map editor locales and run `make gen` (see `web/shared/src/map/README.md`).
