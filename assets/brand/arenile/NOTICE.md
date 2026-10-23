# Arenile brand kit — notice

© Coste360 SRL. All rights reserved.

These files are the **Arenile** trademark assets (name, "A" symbol with sand dune and wave, app icon and
logotypes, brand kit **v2.0**). They are **not** covered by the code licence of the repository (Business Source
License 1.1, later AGPL-3.0-or-later): neither licence grants any right on them. They ship in the repository
only because the official software displays them (admin panel, public booking page, emails, desktop app,
documentation). Use them only as described in [TRADEMARK.md](../../../TRADEMARK.md); forks must remove or
replace them.

The assets were created by Coste360 SRL with the assistance of AI tools (manual vector redraw of an approved,
AI-assisted concept, see `README.md`). Their protectability and the trademark filings are **to be reviewed by
counsel**.

## Contents

- **Brand kit v2.0** supplied by Coste360 SRL and adopted on 2026-10-23 (ADR 0018), replacing kit v1.0:
  `arenile-*.svg`, `arenile-*.png`, `arenile-tokens.css`, `manifest.json`, `README.md`. The logo SVGs keep
  live `<text>` in Titillium Web so they stay editable.
- **Polish of kit v2.0** (approved by the owner, same design): uniform 22-unit gap (on the 1024 grid) between
  the wave and the legs of the "A" (the left-leg cut follows the wave's top curve, the right-leg cut and the
  top of the right foot are offsets of the wave), the crooked notch on the inner edge of the right foot
  removed (the foot continues the leg's inner edge), the symbol optically centred in the tile, and new
  light-surface variants with a navy "A" (`arenile-symbol-color.svg` and the logos without suffix; the
  supplied sand-"A" colours are the `-dark` variants, the supplied `arenile-symbol-color.svg` is now
  `arenile-symbol-color-dark.svg`).
- **Derived files**, generated from the kit by `scripts/brand/build-brand.mjs` (do not edit by hand, rerun
  the script when the kit changes, then `scripts/desktop/gen-icons.sh`):
  - `outlined/`: the kit logos and wordmarks with their text converted to outlines (render the same without
    the font, e.g. in `<img>`), single-colour `-white` / `-navy` logos and `arenile-name-{navy,white}.svg`;
  - `png/`: raster exports for favicons, PWA and Apple touch icons, the maskable icon, the email mark, the
    desktop tray (colour tile and macOS menu bar template), the macOS icon and the social (Open Graph) image;
  - the kit PNG exports at the top level (`arenile-app-icon-*.png`, `arenile-symbol-color*-1024.png`).

## Font

The text was outlined with **Titillium Web** (© 2009-2011 Accademia di Belle Arti di Urbino and students of
MA course of Visual design, licensed under the [SIL Open Font License 1.1](https://openfontlicense.org),
SPDX `OFL-1.1`). The outlines are artwork produced with the font, which the OFL permits; no font file is
distributed in this directory.
