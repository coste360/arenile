# Coste360 brand assets — notice

© Coste360 SRL. All rights reserved.

These files are the **Coste360** / **Coste360.it** company trademark assets (compass-and-waves icon and
logotype). They are **not** covered by the code licence of the repository (Business Source License 1.1,
later AGPL-3.0-or-later): neither licence grants any right on them. They ship in the repository only because
the official software shows its maker (login and first-run pages of the panel, sidebar, documentation site).
Use them only as described in [TRADEMARK.md](../../../TRADEMARK.md); forks must remove them.

Parts of the brand material were created with the assistance of AI tools; protectability and trademark
filings are **to be reviewed by counsel**.

## Contents

- **Original company files** (kept exactly as supplied): `icon-primary.svg`, `icon-white.svg`,
  `icon-*.png`, `logo-{black,primary,white}.png` (781x312), `logo-full-*.png` (981x140, with the tagline).
- **Company SVG lockup** (supplied and approved by the owner on 2026-10-27): `logo.svg`, mark and
  "Coste360.it" outlined, black. It was vectorised from the company logo with an AI tool (QuiverAI Arrow,
  see the comment in the file) and is kept exactly as supplied; it falls under the counsel review above.
  The only derived files are `logo-primary.svg` (fill #0c2442, the company primary) and `logo-white.svg`
  (fill #fff), which change only the fill of the `content_group` group. The lockup is never redrawn.
- **Used by the apps:** the panel uses `logo-{primary,white}.svg`, copied unchanged into
  `web/shared/src/brand/` (`Brand.test.tsx` keeps the panel copies identical and checks the fills); the
  docs site still uses `logo-{primary,white}.png` in `docs-site/public/brand/` (ADR 0018, 2026-10-27
  amendment).
