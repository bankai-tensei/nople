; The installer of Nople for Windows, for Inno Setup 6. build.ps1 builds
; nople.exe and compiles this; by hand:
;
;   go build -ldflags="-H windowsgui -X main.version=0.1.0" -o dist\windows\nople.exe .
;   ISCC /DAppVersion=0.1.0 packaging\windows\nople.iss
;
; It installs for the current user, without administrator rights, into
; %LOCALAPPDATA%\Programs\Nople; the first page also offers to install for
; all users. Explorer's "Open with Nople" and the file types come from
; nople.exe -register, and leave with nople.exe -unregister.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#define AppName "Nople"
#define AppExe "nople.exe"
#define Root "..\.."

[Setup]
; The AppId ties versions together, so that a new one upgrades the old.
; Never change it.
AppId={{6F3C2A1E-8B4D-4E7A-9C51-2D0B7E9F4A63}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=bankai-tensei
AppPublisherURL=https://github.com/bankai-tensei/nople
AppSupportURL=https://github.com/bankai-tensei/nople/issues
AppUpdatesURL=https://github.com/bankai-tensei/nople/releases
VersionInfoVersion={#AppVersion}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile={#Root}\LICENSE
SetupIconFile={#Root}\assets\icon.ico
UninstallDisplayIcon={app}\{#AppExe}
UninstallDisplayName={#AppName}
WizardStyle=modern
Compression=lzma2/max
SolidCompression=yes
; The environment changes when the installer puts Nople on PATH.
ChangesEnvironment=yes
; The associations change with -register.
ChangesAssociations=yes
CloseApplications=yes
OutputDir={#Root}\dist
OutputBaseFilename=nople-{#AppVersion}-windows-x64-setup

[Languages]
Name: "en"; MessagesFile: "compiler:Default.isl"
Name: "ja"; MessagesFile: "compiler:Languages\Japanese.isl"

[CustomMessages]
en.ShellGroup=Integration:
ja.ShellGroup=統合:
en.ShellTask=Add "Open with Nople" to Explorer, and Nople to "Open with" for text and source files
ja.ShellTask=エクスプローラーに「Nople で開く」を追加し、テキストとソースのファイルの「プログラムから開く」に Nople を加える
en.PathTask=Add Nople to PATH (the nople command in a new terminal)
ja.PathTask=PATH に追加する（新しく開いたターミナルで nople コマンドが使える）

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "shell"; Description: "{cm:ShellTask}"; GroupDescription: "{cm:ShellGroup}"
Name: "path"; Description: "{cm:PathTask}"; GroupDescription: "{cm:ShellGroup}"

[Files]
Source: "{#Root}\dist\windows\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Root}\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[Run]
; An upgrade that drops the task takes out what an earlier one put in.
Filename: "{app}\{#AppExe}"; Parameters: "-register"; Flags: runhidden waituntilterminated; Tasks: shell
Filename: "{app}\{#AppExe}"; Parameters: "-unregister"; Flags: runhidden waituntilterminated; Tasks: not shell
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\{#AppExe}"; Parameters: "-unregister"; Flags: runhidden waituntilterminated; RunOnceId: "Unregister"

[Code]
// PATH: the user's for a per-user install, the machine's for one for all
// users. The installer adds {app} once, and the uninstaller takes it out.

function EnvRoot: Integer;
begin
  if IsAdminInstallMode then Result := HKEY_LOCAL_MACHINE else Result := HKEY_CURRENT_USER;
end;

function EnvKey: String;
begin
  if IsAdminInstallMode then
    Result := 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
  else
    Result := 'Environment';
end;

// PathHas reports whether the list of folders path, separated by ";", has
// dir, as Windows compares them: without regard to case or a last "\".
function PathHas(Path, Dir: String): Boolean;
begin
  Result := Pos(';' + Uppercase(RemoveBackslashUnlessRoot(Dir)) + ';',
    ';' + Uppercase(Path) + ';') > 0;
  if not Result then
    Result := Pos(';' + Uppercase(AddBackslash(Dir)) + ';', ';' + Uppercase(Path) + ';') > 0;
end;

procedure AddToPath(Dir: String);
var
  Path: String;
begin
  if not RegQueryStringValue(EnvRoot, EnvKey, 'Path', Path) then Path := '';
  if PathHas(Path, Dir) then exit;
  if (Path <> '') and (Copy(Path, Length(Path), 1) <> ';') then Path := Path + ';';
  RegWriteExpandStringValue(EnvRoot, EnvKey, 'Path', Path + Dir);
end;

procedure RemoveFromPath(Dir: String);
var
  Path, Rest, Item, Kept: String;
  P: Integer;
begin
  if not RegQueryStringValue(EnvRoot, EnvKey, 'Path', Path) then exit;
  Rest := Path;
  Kept := '';
  while Rest <> '' do begin
    P := Pos(';', Rest);
    if P = 0 then begin
      Item := Rest;
      Rest := '';
    end else begin
      Item := Copy(Rest, 1, P - 1);
      Rest := Copy(Rest, P + 1, Length(Rest));
    end;
    if (Item <> '') and (CompareText(RemoveBackslashUnlessRoot(Item), RemoveBackslashUnlessRoot(Dir)) <> 0) then begin
      if Kept <> '' then Kept := Kept + ';';
      Kept := Kept + Item;
    end;
  end;
  if Kept <> Path then RegWriteExpandStringValue(EnvRoot, EnvKey, 'Path', Kept);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    if WizardIsTaskSelected('path') then
      AddToPath(ExpandConstant('{app}'))
    else
      RemoveFromPath(ExpandConstant('{app}'));
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then RemoveFromPath(ExpandConstant('{app}'));
end;
