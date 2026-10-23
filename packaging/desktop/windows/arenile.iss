; Inno Setup 6 script of the Arenile desktop installer (spec 29.1: Windows ".exe con installer").
;
; Built by scripts/desktop/package-windows.sh (never by hand in CI):
;   ISCC.exe /DAppVersion=1.4.0 /DAppVersionTag=v1.4.0 /DSourceExe=...\arenile-desktop.exe
;            /DRepoRoot=... /DOutputDir=... /DOutputName=Arenile_v1.4.0_windows_amd64_setup /DArch=amd64
;            [/DSign "/Ssigntool=signtool.exe sign ... $f"] arenile.iss
;
; Why Inno Setup and not WiX: one readable script, preinstalled on the GitHub
; windows runners, it produces the single setup .exe the spec asks for, installs
; per user without administrator rights (the PC of the cash desk often has none),
; ships Italian/English/German/French wizard translations and closes a running
; launcher through the Restart Manager on upgrade. WiX (MSI) is the better fit for
; GPO/Intune mass deployment, which is not the target of the desktop edition.
;
; The data folder %APPDATA%\Arenile (database, backups, secret key fallback) is
; NEVER touched by the installer nor by the uninstaller: data survive reinstalls.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef AppVersionTag
  #define AppVersionTag AppVersion
#endif
#ifndef RepoRoot
  #define RepoRoot "..\..\.."
#endif
#ifndef Arch
  #define Arch "amd64"
#endif
#ifndef SourceExe
  #define SourceExe RepoRoot + "\dist\desktop\bin\windows_" + Arch + "\arenile-desktop.exe"
#endif
#ifndef OutputDir
  #define OutputDir RepoRoot + "\dist\desktop"
#endif
#ifndef OutputName
  #define OutputName "Arenile_" + AppVersionTag + "_windows_" + Arch + "_setup"
#endif

[Setup]
; AppId identifies the product for upgrades and uninstall: never change it.
AppId={{30943EB4-A7B7-4773-B759-AD976AF1CD49}
AppName=Arenile
AppVersion={#AppVersion}
AppVerName=Arenile {#AppVersionTag}
AppPublisher=Coste360 SRL
AppPublisherURL=https://www.coste360.it/
AppSupportURL=https://arenile.app
AppCopyright=Coste360 SRL - BSL 1.1 (BUSL-1.1)
VersionInfoVersion={#AppVersion}.0
VersionInfoProductTextVersion={#AppVersionTag}
VersionInfoDescription=Arenile - installazione
DefaultDirName={autopf}\Arenile
DefaultGroupName=Arenile
DisableProgramGroupPage=yes
; Per-user install by default, no UAC prompt; an administrator can still choose "all users".
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
; amd64: x64 PCs, and Windows 11 on Arm through the x64 emulation; arm64: native
; build for Snapdragon PCs (Surface Pro), offered next to it on the portal.
#if Arch == "arm64"
ArchitecturesAllowed=arm64
ArchitecturesInstallIn64BitMode=arm64
#else
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#endif
MinVersion=10.0
OutputDir={#OutputDir}
OutputBaseFilename={#OutputName}
SetupIconFile={#RepoRoot}\packaging\desktop\icons\arenile.ico
UninstallDisplayIcon={app}\arenile-desktop.exe
UninstallDisplayName=Arenile
WizardStyle=modern
Compression=lzma2/max
SolidCompression=yes
CloseApplications=yes
RestartApplications=no
ShowLanguageDialog=auto
TimeStampsInUTC=yes
#ifdef Sign
SignTool={#Sign}
SignedUninstaller=yes
#endif

[Languages]
Name: "it"; MessagesFile: "compiler:Languages\Italian.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"
Name: "de"; MessagesFile: "compiler:Languages\German.isl"
Name: "fr"; MessagesFile: "compiler:Languages\French.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "arenile-desktop.exe"; Flags: ignoreversion
Source: "{#RepoRoot}\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Arenile"; Filename: "{app}\arenile-desktop.exe"; WorkingDir: "{app}"
Name: "{autodesktop}\Arenile"; Filename: "{app}\arenile-desktop.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\arenile-desktop.exe"; Description: "{cm:LaunchProgram,Arenile}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; Autostart is managed by the app; contract with the launcher: HKCU Run value named "Arenile". Removed with the program.
Filename: "{sys}\reg.exe"; Parameters: "delete HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v Arenile /f"; Flags: runhidden; RunOnceId: "RemoveAutostart"
