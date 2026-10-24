# Beach map kit "Classico" (v2.1) — provenance

© Coste360 SRL, original artwork created by Coste360 with the help of an image-generation model;
distributed with Arenile under the repository licence (BSL 1.1).

- Owner: Coste360 SRL. The artwork is original and not derived from third-party icon sets.
- Licence: the same as the repository (Business Source License 1.1, SPDX `BUSL-1.1`, converting to
  AGPL-3.0-or-later as described in `LICENSE` and `docs/adr/0007-licenza-bsl.md`).
- Delivered: v2.1.0, October 2026 (ADR 0012). Since v3.0 (ADR 0024) it is the alternative kit
  `classic`, chosen per establishment; the default is `modern` (`assets/map-kits/modern`).
- Source of truth: the files in this directory (`svg/*.svg`, `manifest.json`, `beach-tokens.css`,
  `catalogo.html`, `README.md`) are kept exactly as supplied. Never edit them to fix the app: the
  generator `web/scripts/gen-map-kit.mjs` (run by `make gen`) reads them, sanitises them and writes
  `web/shared/src/map/kit/generated/` and `internal/domain/mapping/kitassets_gen.go`. Adjustments
  the app needs (for example the axis-aligned `ombrellone_quadrato`, or the `asset` manifest type
  folded into `bookable`/`structure`) live in the generator and are documented in
  `docs/adr/0012-kit-mappa.md`.
- The booking status is drawn by the app as a halo under the art (`halo` profile); the art is never
  recoloured.
