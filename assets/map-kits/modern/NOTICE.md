# Beach map kit "Moderno" (v3.0, stile pieno monocolore) — provenance

© Coste360 SRL, original artwork created by Coste360; distributed with Arenile under the
repository licence (BSL 1.1).

- Owner: Coste360 SRL. The artwork is original and not derived from third-party icon sets.
- Licence: the same as the repository (Business Source License 1.1, SPDX `BUSL-1.1`, converting to
  AGPL-3.0-or-later as described in `LICENSE` and `docs/adr/0007-licenza-bsl.md`).
- Delivered: v3.0.0, October 2026 (ADR 0024). It is the default kit `modern`; the v2.1 kit stays
  available as `classic` (`assets/map-kits/classic`). Same 107 asset ids, so saved maps work with
  either kit.
- Source of truth: the files in this directory (`svg/*.svg`, `manifest.json`, `beach-tokens.css`,
  `catalogo.html`, `README.md`) are kept exactly as supplied. The kit's own build script and sources
  were not imported: the delivered SVGs are the source. Never edit them to fix the app: the
  generator `web/scripts/gen-map-kit.mjs` (run by `make gen`) reads them, validates them against the
  v3 rules (`fill` profile: elements `g/path/rect/circle/ellipse` only, colours only
  `var(--beach-*, fallback)`, indicator corner x 92–120, y 0–28 empty except `mare` and `sabbia`,
  every bookable asset painting its status with `--beach-state`), sanitises them and writes
  `web/shared/src/map/kit/generated/` and `internal/domain/mapping/kitassets_gen.go`.
- The booking status colour is set by the app on `--beach-state` from its own status palette
  (`--status-*` tokens), not from the `--beach-state-*` tokens of `beach-tokens.css`, which are
  documented here only as the designer's reference.
