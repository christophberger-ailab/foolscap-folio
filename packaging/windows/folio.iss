; Inno Setup script for folio's Windows setup program,
; folio_<tag>_windows_setup.exe. release.yml builds it on a Windows runner:
;
;   ISCC.exe /DAppVersion=0.12.3 /DBinDir=<dir> packaging\windows\folio.iss
;
; BinDir holds amd64\folio.exe and arm64\folio.exe; the setup program carries
; both and installs the one for the machine. Paths are relative to this file.
;
; The install is per user, into %LOCALAPPDATA%\Programs\Foolscap Folio, and
; needs no administrator rights. That folder is the user's own, so folio
; upgrade and the Update button in folio web replace folio.exe there
; (spec-upgrade.yaml SECURITY 7-1); the version Windows lists under
; Installed apps is the one the setup program installed.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef BinDir
  #define BinDir "..\..\build"
#endif

#define AppName "Foolscap Folio"
#define AppURL "https://github.com/christophberger-ailab/foolscap-folio"

[Setup]
; The AppId ties every version together: a newer setup program updates the
; install in place. Never change it.
AppId={{4107A2E9-E1DB-488E-AFF0-EFAB70E747BB}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=Christoph Berger
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
VersionInfoVersion={#AppVersion}
DefaultDirName={autopf}\{#AppName}
DisableDirPage=auto
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
; x64compatible takes in Windows on Arm too, which runs x64 programs; the
; [Files] section installs the native arm64 binary there.
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
ChangesEnvironment=yes
LicenseFile=..\..\LICENSE
SetupIconFile=folio.ico
UninstallDisplayIcon={app}\folio.ico
UninstallDisplayName={#AppName}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
OutputDir=..\..\installer
OutputBaseFilename=folio_v{#AppVersion}_windows_setup

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "german"; MessagesFile: "compiler:Languages\German.isl"

[Files]
Source: "{#BinDir}\amd64\folio.exe"; DestDir: "{app}"; Check: not IsArm64; Flags: ignoreversion
Source: "{#BinDir}\arm64\folio.exe"; DestDir: "{app}"; Check: IsArm64; Flags: ignoreversion
Source: "folio.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[UninstallDelete]
; What an update on Windows leaves behind until folio next starts.
Type: files; Name: "{app}\folio.exe.old"

[Icons]
; folio web in a console window, which shows the address to open in the
; browser; closing the window stops it. It starts in Documents, and the Open
; button picks a project from there.
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\folio.exe"; Parameters: "web"; WorkingDir: "{userdocs}"; IconFilename: "{app}\folio.ico"; Comment: "Start the Foolscap Folio web UI"

[Run]
Filename: "{app}\folio.exe"; Parameters: "web"; WorkingDir: "{userdocs}"; Description: "Start {#AppName}"; Flags: postinstall nowait skipifsilent

[Code]
// folio is a command-line program, so its folder goes on the user's PATH
// (HKCU, no administrator rights needed) and comes off it on uninstall.
// ChangesEnvironment tells Windows, so new console windows see it.

const
  EnvKey = 'Environment';

function OnPath(Path, Dir: string): Integer;
begin
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Path) + ';');
end;

procedure AddToPath(Dir: string);
var
  Path: string;
begin
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then
    Path := '';
  if OnPath(Path, Dir) > 0 then
    exit;
  if (Path <> '') and (Path[Length(Path)] <> ';') then
    Path := Path + ';';
  RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Path + Dir);
end;

procedure RemoveFromPath(Dir: string);
var
  Path: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then
    exit;
  P := OnPath(Path, Dir);
  if P = 0 then
    exit;
  // P is the position of ';Dir;' in ';Path;': drop ';Dir' and the
  // semicolons added around Path.
  Path := ';' + Path + ';';
  Delete(Path, P, Length(Dir) + 1);
  RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Copy(Path, 2, Length(Path) - 2));
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    AddToPath(ExpandConstant('{app}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;
