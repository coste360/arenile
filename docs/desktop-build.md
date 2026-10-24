# Building the desktop packages

The desktop edition (spec 29.1, milestone M6) is the launcher `cmd/arenile-desktop`: the
same core as `arenile serve`, with the admin panel and the public page embedded, shown in the
app's own window (no browser needed) plus a tray icon. This page explains how to build its packages locally and in CI, and how signing works.

Distribution rule: the packages are **never** uploaded as GitHub release assets nor as Actions
artifacts. This repository is public, and an Actions artifact of a public repository can be
downloaded by anyone with a GitHub account. In this repository `desktop.yml` only builds and
smoke tests the packages (pull requests and `workflow_dispatch`); the signed packages are
built in the private build repository with the same scripts and published on
[arenile.app/download](https://arenile.app/download/). GitHub carries the source code and these instructions only.

| OS | Package | Tool | Runner |
| --- | --- | --- | --- |
| Windows 10/11 x64 | `Arenile_<tag>_windows_amd64_setup.exe` | Inno Setup 6 | `windows-latest` |
| Windows 11 on Arm (Snapdragon, Surface Pro) | `Arenile_<tag>_windows_arm64_setup.exe` | Inno Setup 6, llvm-mingw | `windows-11-arm` |
| macOS 13+ (Apple Silicon and Intel) | `Arenile_<tag>_macos_universal.dmg` | `lipo`, `hdiutil`, `codesign`, `notarytool` | `macos-latest` |
| macOS 13+ Apple Silicon only | `Arenile_<tag>_macos_arm64.dmg` (about half the size) | same | `macos-latest` |
| macOS 13+ Intel only | `Arenile_<tag>_macos_x86_64.dmg` (about half the size) | same | `macos-latest` |
| Linux x86_64 | `Arenile_<tag>_linux_x86_64.AppImage` | `appimagetool` (pinned) | `ubuntu-latest` |

Each package has a `.sha256` file next to it, and the portal bundle has a `checksums.txt` file
that covers every file. A package without a valid publisher signature always has `-unsigned` in
its name. On macOS, a signed package that was not notarized has `-unnotarized` instead, and one
whose notarization Apple had not finished within `NOTARY_TIMEOUT` has `-notarization-pending`
(see [Notarization pending](#notarization-pending)). None of these may be published on
arenile.app/download as a stable release.

## Files

```
packaging/desktop/
  icons/      arenile.svg / arenile.ico / arenile.icns / arenile.png, generated from the brand kit
              (assets/brand/arenile) by scripts/desktop/gen-icons.sh, committed
  windows/    arenile.iss (Inno Setup), arenile.rc.in (icon + version info), arenile.exe.manifest
  macos/      Info.plist.in, entitlements.plist (hardened runtime, intentionally empty)
  linux/      arenile.desktop, AppRun, it.coste360.arenile.metainfo.xml, apt-packages.txt
scripts/desktop/
  common.sh   version, ldflags, naming, checksums (sourced by the others)
  build.sh    launcher binary only (any OS, cross-compiles when no build tags are set)
  package-macos.sh, package-windows.sh, package-linux.sh, gen-icons.sh
.github/workflows/desktop.yml   native jobs; called by release.yml, also runs on PRs touching the desktop code
```

## Building locally

Prerequisites: the `make setup` toolchain (Go from `go.mod`, Node 22). The scripts run
`make web-build` first, because the bundle must contain the SPAs. Set `SKIP_WEB_BUILD=1`
when `internal/http/static/{admin,booking}` are already filled. Everything is written to
`dist/desktop/`, which git ignores.

```bash
make desktop-build                    # launcher for this machine -> dist/desktop/bin/<os>_<arch>/
make desktop-package-macos            # macOS: universal Arenile.app + .dmg
MACOS_ARCHS=arm64 make desktop-package-macos   # thin Apple Silicon .dmg (_macos_arm64)
MACOS_ARCHS=amd64 make desktop-package-macos   # thin Intel .dmg (_macos_x86_64)
make desktop-package-linux            # Linux: AppImage (downloads the pinned appimagetool, checks sha256)
make desktop-package-windows          # Windows, in Git Bash: exe + Inno Setup installer
VERSION=v1.4.0 make desktop-package-macos     # version label (default: scripts/version.sh, e.g. 0.6.0-dev.187+835d645)
DESKTOP_TAGS=desktop make desktop-package-macos   # tray build (CGO), as in CI
```

- **Build tags.** With `DESKTOP_TAGS` empty, the launcher has no CGO and no tray (it is
  headless and opens the browser). It cross-compiles from any OS, so it is handy for testing
  the packaging. The published packages use `DESKTOP_TAGS=desktop`: on macOS and Windows
  this turns CGO on (panel window, tray, remote-mode webview: `-version` prints `(window)`)
  and the build must run on the native OS; on Linux the tray build stays CGO-free (tray over
  D-Bus, panel in the browser) and cross-compiles. On macOS, the host
  clang builds both slices (`clang -arch x86_64` and `-arch arm64`), and `lipo` merges them
  into one universal binary.
- **macOS.** You need only the Xcode command line tools. Without a Developer ID, the `.app`
  gets an ad-hoc signature (it runs on the Mac that built it) and the `.dmg` is named
  `-unsigned`. Gatekeeper blocks it on other Macs: right click, then *Open*.
- **Windows.** You need Git Bash, MinGW (`gcc` and `windres`, for the icon and version
  resources and for CGO) and Inno Setup 6 (`choco install innosetup`). For the arm64 package
  set `WINDOWS_GOARCH=arm64` and put llvm-mingw's `bin` (the `ucrt-aarch64` build, which runs
  natively on Windows on Arm) on the `PATH` with `CC=aarch64-w64-mingw32-clang`
  `CXX=aarch64-w64-mingw32-clang++`: it provides the C/C++ compiler for CGO (webview_go is
  C++) and `aarch64-w64-mingw32-windres` for the resources. cgo cannot use MSVC. Without `windres` the
  `.exe` has no icon, and the script prints a warning. The stages can be run one at a time:
  `scripts/desktop/package-windows.sh binary|sign FILE|installer|checksums`.
- **Linux.** Any distribution with `curl`, `file` and `sha256sum`. FUSE is not needed (the
  script sets `APPIMAGE_EXTRACT_AND_RUN=1`). No system library: the Linux launcher is a
  static binary (`packaging/desktop/linux/apt-packages.txt` is empty on purpose).
- **Icons.** Run `scripts/desktop/gen-icons.sh` (ImageMagick 7 and macOS `iconutil`) only
  when the brand mark changes, then commit the outputs.

Smoke test: `dist/desktop/macos/Arenile.app/Contents/MacOS/arenile-desktop -version` prints
`arenile-desktop <tag> (headless|tray|window)`.

## The panel window (spec 29.1)

With the `desktop` tag on macOS and Windows (`internal/desktop/shell`, ADR 0015 point 19):

- The panel opens in the system webview (WKWebView, WebView2), title "Arenile", minimum
  960x600, size, position, maximized state and zoom saved in `<data dir>/window.json`.
- The tray menu keeps *Apri pannello* (brings the window to the front) and adds *Apri nel
  browser*; `-browser` starts with the panel in the browser (tray only, no Dock icon),
  `-no-browser` and `-no-tray` keep their old meaning (no window). Windows without the
  WebView2 runtime (rare: Windows 11 ships it) falls back to the browser.
- A second launch on the same data folder sends a message to the running one (loopback
  listener, token in `instance.json`) and exits: the window comes to the front.
- macOS: Dock icon and application menu (*Arenile*: Informazioni, Impostazioni ⌘, which
  opens Configurazione → App desktop, Nascondi, Esci ⌘Q; *Modifica*; *Vista*: Ricarica ⌘R,
  zoom ⌘+ ⌘- ⌘0, Schermo intero, Apri nel browser; *Finestra*). The close button and ⌘W hide
  the window; the Dock icon or the menu bar icon shows it again. ⌘Q, *Esci* and the Dock's
  quit stop the app in order (exit backup, LAN mode off). Downloads go to `~/Downloads`
  (unique name, notification, Downloads stack bounce); `window.print` opens the system print
  panel; links to other sites open in the default browser.
- Windows: closing the window quits Arenile unless *Configurazione → App desktop → Alla
  chiusura della finestra resta attivo nella barra di sistema* is on (`keep_running_in_tray`,
  default off; turn it on for a cash desk PC that serves the LAN mode). WebView2 handles
  downloads (Downloads folder and its flyout), printing (print preview), zoom (Ctrl +/-/0),
  reload (F5) and the edit shortcuts by itself. The window icon comes from resource 32512 of
  the exe (`arenile.rc.in`).
- Linux: no window (WebKitGTK 4.0, see below); the panel opens in the default browser.

## Versioning and reproducibility

- The desktop packages use the same tag as the server edition. `release.yml` passes
  `github.ref_name` to `desktop.yml`, and the version is injected with
  `-X main.version=<tag>`.
- The numeric fields (`CFBundleShortVersionString`, the Windows `FILEVERSION` and the Inno
  `AppVersion`) take `MAJOR.MINOR.PATCH` from the tag (`v1.4.0-rc.1` gives `1.4.0`).
- Builds are reproducible-ish: `-trimpath`, `-buildvcs=false`, `-ldflags "-s -w -buildid="`,
  and `SOURCE_DATE_EPOCH` set to the commit time for the AppImage squashfs. The AppImage tool
  and runtime are pinned and verified by sha256, never "continuous". Code signatures carry a
  timestamp, so signed files are not byte-identical between builds.

## CI

`desktop.yml` runs four native jobs after a shared frontend build:

1. `macos` (`macos-latest`, arm64) is a matrix of three packages: `universal`
   (`MACOS_ARCHS="arm64 amd64"`, artifact `desktop-macos`), `arm64` (`desktop-macos-arm64`)
   and `x86_64` (`desktop-macos-x86_64`). The thin ones are about half the size of the
   universal one, for arenile.app/download and for sending by hand. Each builds the binary, the `.app`
   and the `.dmg`, signs, notarizes and staples when the secrets are present, then checks the
   slices with `lipo -archs` and runs `-version` (the Intel build under Rosetta 2 when the
   runner has it). When notarization is still pending, the signed dmg is uploaded anyway and
   a last step fails the job (see [Notarization pending](#notarization-pending)).
2. `windows` is a matrix: `amd64` on `windows-latest` (MinGW gcc of the image) and `arm64`
   on the GitHub-hosted `windows-11-arm` runner, where it downloads llvm-mingw (version and
   sha256 pinned in the workflow) for CGO. Each builds the exe with its resources, signs it
   (PFX or Azure), builds the Inno Setup installer (`/DArch=amd64|arm64`), signs it, writes
   the checksums and runs `-version` natively, checking `(window)`, `GOARCH` and
   `CGO_ENABLED=1` in the build info.
3. `linux` (`ubuntu-latest`) builds the AppImage and runs it with `-version`. The binary
   is CGO-free, so the runner's glibc is irrelevant and the AppImage runs on any current
   distribution (Ubuntu 22.04/24.04, Debian 12/13, Fedora) without extra libraries.

In this repository the packages are built and smoke tested but never uploaded, also when a
maintainer runs the workflow by hand (`workflow_dispatch`). `release.yml` no longer calls it:
its `portal-bundle` job collects the server binaries, writes `checksums.txt` and, when the
`PORTAL_UPLOAD_URL` and `PORTAL_UPLOAD_TOKEN` secrets exist, pushes them to the portal intake
endpoint (one `multipart/form-data` POST per file, `version` and `file` fields, Bearer token);
then it deletes the intermediate artifacts of the run.
A failed desktop job does not block the server bundle or the release: the bundle then carries
the server binaries and the desktop packages that were built, with a warning in the job summary, and `desktop.yml` can be re-run
with the same tag. The `release` job still publishes only the notes and `openapi.yaml`.

## Signing

Every signing step depends on whether its secrets exist. If they are missing, the build still
succeeds and the package is named `-unsigned`. Store the secrets in the GitHub environment or
repository secrets of `coste360/arenile`. Pull requests from forks never see them.

### macOS: Developer ID, notarization and stapling

| Secret | Content |
| --- | --- |
| `APPLE_CERTIFICATE_P12_BASE64` | "Developer ID Application" certificate with its private key, exported as `.p12`, then `base64` |
| `APPLE_CERTIFICATE_PASSWORD` | password of the `.p12` |
| `APPLE_SIGNING_IDENTITY` | optional, e.g. `Developer ID Application: Coste360 SRL (TEAMID)` (default: the first one in the `.p12`) |
| `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID` | App Store Connect API key (Users and Access → Integrations → Team keys, role *Developer*) |
| `APPLE_API_KEY_P8_BASE64` | the `AuthKey_<id>.p8` file, `base64` |

The flow in `package-macos.sh`:

1. Import the `.p12` into a temporary keychain, which is deleted at the end.
2. Run `codesign --options runtime --timestamp --entitlements entitlements.plist` on the
   `.app`. The hardened runtime is required for notarization, and the entitlements are empty
   because a Go binary needs no exceptions.
3. Zip the `.app` with `ditto`, submit it with `notarytool submit`, wait for it up to
   `NOTARY_TIMEOUT` (default 90m) with `notarytool wait`, then
   `stapler staple Arenile.app`. The `.app` is stapled on its own so that it opens offline
   once it has been copied out of the `.dmg`, as in the acceptance test on a PC with no
   connection.
4. Run `hdiutil create` on the `.dmg`, sign it, notarize it and staple it.

If notarization is rejected, the script prints the `notarytool log` and the job fails.

#### Notarization pending

The first submissions of a new Apple team can take hours. When Apple has not answered within
`NOTARY_TIMEOUT`, `package-macos.sh` does not throw the build away: it still writes the
signed dmg, named `-notarization-pending` (if the `.app` was still pending, the dmg is
submitted without waiting), and a `<dmg>.notarization.txt` file with the submission ids.
The build writes the ids in the job summary and then fails the job, so that nobody publishes
the package by mistake.

To finish by hand on a Mac, with the App Store Connect API key (`AuthKey_<KEY_ID>.p8`: keep it
outside the repository, never print or commit it):

```bash
KEY=/path/to/AuthKey_<KEY_ID>.p8
xcrun notarytool history --key "$KEY" --key-id <KEY_ID> --issuer <ISSUER_ID>   # recent submissions and status
xcrun notarytool info <submission-id> --key "$KEY" --key-id <KEY_ID> --issuer <ISSUER_ID>
xcrun notarytool log  <submission-id> --key "$KEY" --key-id <KEY_ID> --issuer <ISSUER_ID>   # if Invalid
```

Once the dmg submission is `Accepted`, staple the ticket (stapling needs no signing key), check
it, drop `-notarization-pending` from the name and write the checksum again:

```bash
xcrun stapler staple Arenile_<tag>_macos_arm64-notarization-pending.dmg
xcrun stapler validate Arenile_<tag>_macos_arm64-notarization-pending.dmg
spctl -a -vvv -t open --context context:primary-signature Arenile_<tag>_macos_arm64-notarization-pending.dmg
mv Arenile_<tag>_macos_arm64-notarization-pending.dmg Arenile_<tag>_macos_arm64.dmg
shasum -a 256 Arenile_<tag>_macos_arm64.dmg > Arenile_<tag>_macos_arm64.dmg.sha256
```

The `.app` inside such a dmg is not stapled itself: Gatekeeper checks its ticket online the
first time it opens. Re-running the job after Apple has accepted gives fully stapled packages.

### Windows: Authenticode

There are two ways to sign, and both are supported.

**A. PFX certificate with signtool.** This is the simplest when Coste360 has an exportable
certificate. Secrets: `WINDOWS_CERTIFICATE_PFX_BASE64` (the `.pfx`, `base64`) and
`WINDOWS_CERTIFICATE_PASSWORD`. The script signs `arenile-desktop.exe` with
`signtool sign /fd SHA256 /tr <RFC 3161> /td SHA256`. It also passes Inno Setup a `SignTool`,
so the setup and the uninstaller embedded in it are signed too. The timestamp server defaults
to DigiCert and can be changed with `WINDOWS_TIMESTAMP_URL`. Note: since June 2023, new
OV and EV certificates must keep the private key on an HSM or token, which in practice cannot
be exported as a PFX. This option suits existing certificates or a cloud HSM that exposes
one.

**B. Azure Artifact Signing (formerly Trusted Signing).** This is the recommended option
for new certificates: there is no key to look after, and the cost is per month. Secrets:
`AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` (a service principal with the
*Artifact Signing Certificate Profile Signer* role), `AZURE_TRUSTED_SIGNING_ENDPOINT` (e.g.
`https://weu.codesigning.azure.net`), `AZURE_TRUSTED_SIGNING_ACCOUNT` and
`AZURE_TRUSTED_SIGNING_PROFILE`. The workflow runs `azure/artifact-signing-action` twice:
on the exe before the installer is built, and on the installer afterwards
(`WINDOWS_SIGNING=external` removes the `-unsigned` label). Limitation: the uninstaller
embedded in the installer is not signed by this route. To sign it as well, configure the
`signtool` + Artifact Signing *dlib* as an Inno `SignTool`.

If both are configured, the PFX wins. Either way, SmartScreen builds reputation over time:
the first downloads of a new certificate may still show a warning.

### Linux

AppImages carry no publisher signature. Integrity comes from the sha256 checksums published
next to the packages. If a signature is needed later, a detached GPG signature
(`gpg --detach-sign`) can be added in `package-linux.sh`, gated on a secret like the others.

## Installed layout and data

- Windows: per user, no administrator rights, in `%LOCALAPPDATA%\Programs\Arenile` (an
  administrator can choose *all users*), with a Start menu entry and an optional desktop icon.
  The uninstaller removes the autostart value `HKCU\...\Run\Arenile`.
- macOS: drag `Arenile.app` to Applications. It is a regular app (Dock icon, application
  menu) with an icon in the menu bar as well; closing the window keeps it running there.
- Linux: make the AppImage executable and run it. The `.desktop` file and icon are inside.
- The data stays in the user folder of the OS (`%APPDATA%\Arenile`,
  `~/Library/Application Support/Arenile`, `~/.local/share/arenile`). No installer or
  uninstaller ever touches it.

## Remote mode from the command line

Besides the pairing form in the app window, a PC can be paired from a terminal
(managed installs, PCs without a camera):

```sh
arenile-desktop -pair 'arenile://pair?d=...'            # text of the QR
arenile-desktop -pair https://lido.example.it -pair-code ABCD-EFGH
arenile-desktop -pair https://192.168.1.20:8443 -pair-code ABCD-EFGH \
  -pair-fingerprint 'B7:B7:...'                           # self-signed instance
```

The token goes to the OS keychain and the data folder switches to remote mode:
the next launch opens the remote panel and starts the fiscal bridge.

## Linux runtime requirements

None: the Linux launcher (headless or tray) is a static binary. The tray icon needs a
StatusNotifierItem host (GNOME with the AppIndicator extension, KDE Plasma, XFCE); without
one the app still runs and the panel opens in the browser. The remote-mode window is the
default browser (or Chrome/Chromium/Edge in kiosk mode) rather than a WebKitGTK webview:
`webview_go` links WebKitGTK 4.0, which Ubuntu 24.04, Debian 13 and Fedora no longer ship.
Pair a Linux PC in remote mode with `arenile-desktop -pair <QR|URL> -pair-code <code>`.

## M6 acceptance on a Mac (Collaudo M6)

Repeatable part of the spec 28 acceptance, without Windows:

```sh
make desktop-package-macos
D="$(mktemp -d)/arenile"
# ARENILE_UPDATE_CHECK=false: nessuna chiamata a updates.coste360.it né avviso durante il collaudo (ADR 0025).
ARENILE_UPDATE_CHECK=false dist/desktop/macos/Arenile.app/Contents/MacOS/arenile-desktop -data-dir "$D" -no-browser &
# instance.json (0600) holds the one-time first-run URL while no owner exists
cd e2e && DESKTOP_OPEN_URL="$(jq -r .open_url "$D/instance.json")" \
  npx playwright test -c playwright.desktop.config.ts
```

`e2e/desktop/first-run.spec.ts` creates the owner, runs the minimal guided setup
and creates the first desk booking, printing the time of each step. Then:
"Backup ora" (or `POST /api/desktop/v1/backups`), `arenile restore --in <backup>`
into a fresh `ARENILE_DATA_DIR`, and compare the row counts. Delete the test
keychain entry afterwards (service `it.coste360.arenile`, account
`secret-key-<first 12 hex of sha256(data dir)>`).

## Known gaps

- The signed packages need the Coste360 certificates (Apple Developer ID plus an App Store
  Connect key, and a Windows Authenticode certificate or Azure account). Until then, CI
  produces `-unsigned` packages.
- The acceptance test (spec 28, M6) must still run on a real Windows PC and on a Mac with
  no connection, with the tray build (the macOS run above used the headless package).
- Moving from the desktop app to an online instance: the secret key stays in the OS
  keychain and cannot be exported yet, so secrets encrypted with it (TOTP) must be set up
  again on the new instance.
- The Windows arm64 job has not run on GitHub yet and the package has not been tried on a
  Snapdragon PC; the Windows window layer (`win_windows.go`) is type-checked for amd64 and
  arm64 but was not run on Windows. Linux aarch64 packages are not built yet;
  `package-linux.sh` already supports `LINUX_GOARCH=arm64` on an arm64 host (`ubuntu-24.04-arm`).
- macOS logout or shutdown while Arenile runs: the app cancels the request, runs its own quit
  (exit backup) and exits; macOS reports that Arenile interrupted the logout, the next try
  succeeds. A Windows logoff ends the process without the exit backup (as in the tray build);
  the daily backup covers it.
- On macOS a top-level navigation of the window to another origin (redirect, script, form)
  is sent to the default browser; on Windows webview_go has no navigation hook, so only links
  and `window.open` are diverted (the panel itself never navigates away).
- Open SSE streams keep `app.Run`'s graceful shutdown waiting up to its timeout (15 s) on
  every edition; the window unloads its page before quitting, a browser tab does not.
