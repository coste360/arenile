# Beach map kits

Illustration kits of the beach map, © Coste360 SRL (see each `NOTICE.md`). Every kit is shipped in
the binary and chosen per establishment (Configurazione → Stabilimento → Mappa, field
`theme.map_kit`; ADR 0024).

| Key       | Name     | Version | Status drawn as                       | Default |
| --------- | -------- | ------- | ------------------------------------- | ------- |
| `modern`  | Moderno  | 3.0.0   | the art's status part (`--beach-state`) | yes     |
| `classic` | Classico | 2.1.0   | a halo under the art                  | no      |

Each directory holds `svg/` (one file per asset, viewBox `0 0 120 120`), `manifest.json`,
`beach-tokens.css`, the designer's `README.md` and `catalogo.html`, and `NOTICE.md`. All kits must
have the same asset ids, categories and types, so switching kit never breaks a saved map.

The files are kept as supplied. `make gen` runs `web/scripts/gen-map-kit.mjs`, which validates and
sanitises every kit and writes `web/shared/src/map/kit/generated/` and
`internal/domain/mapping/kitassets_gen.go`; it fails, naming the file and the rule, on anything the
kit's profile does not allow. To add a kit: create `assets/map-kits/<key>/`, add it to `KITS` in the
generator with its profile, add its name to the locales and run `make gen`.
