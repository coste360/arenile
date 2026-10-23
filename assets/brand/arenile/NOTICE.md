# Arenile brand kit — notice

© Coste360 SRL. All rights reserved.

These files are the **Arenile** trademark assets (name, "A" symbol with sand and wave, app icon, logotypes
and the official lockup **"Arenile — Coste360.it"**). They are **not** covered by the code licence of the
repository (Business Source License 1.1, later AGPL-3.0-or-later): neither licence grants any right on them.
They ship in the repository only because the official software displays them (admin panel, public booking
page, emails, desktop app, documentation). Use them only as described in [TRADEMARK.md](../../../TRADEMARK.md);
forks must remove or replace them.

The assets were created by Coste360 SRL with the assistance of AI tools (vector reinterpretation of an
AI-generated concept, see `README.md`). Their protectability and the trademark filings are **to be reviewed by
counsel**.

## Contents

- **Original kit v1.0** (kept exactly as supplied, never edit to fix the app): `arenile-*.svg`,
  `arenile-*.png`, `arenile-tokens.css`, `manifest.json`, `README.md`, `anteprima.html`,
  `anteprima-coste360it.svg`. The logo SVGs use live `<text>` in Titillium Web.
- **Derived files**, generated from the kit by `scripts/brand/build-brand.mjs` (do not edit by hand, rerun
  the script when the kit changes):
  - `outlined/`: the kit logos with their text converted to outlines (render the same without the font,
    e.g. in `<img>`), plus `arenile-logo-header-dark.svg` and `arenile-name-{navy,white}.svg`;
  - `png/`: raster exports for favicons, PWA and Apple touch icons, the maskable icon, the email footer
    mark, the desktop tray and macOS icon, the social (Open Graph) image.

## Font

The text was outlined with **Titillium Web** (© 2009-2011 Accademia di Belle Arti di Urbino and students of
MA course of Visual design, licensed under the [SIL Open Font License 1.1](https://openfontlicense.org),
SPDX `OFL-1.1`). The outlines are artwork produced with the font, which the OFL permits; no font file is
distributed in this directory.
