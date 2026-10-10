#ifndef AppVersion
  #define AppVersion "2.0.3"
#endif
#ifndef AppExe
  #define AppExe "..\..\dist\Scaner-" + AppVersion + "-windows-amd64.exe"
#endif

[Setup]
AppId={{A41696F3-6D01-4D2F-95E6-6D27A5A30C03}
AppName=Scaner
AppVersion={#AppVersion}
AppPublisher=Scaner
AppPublisherURL=https://github.com/salehsayyadi/scaner
DefaultDirName={localappdata}\Programs\Scaner
DefaultGroupName=Scaner
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir=..\..\dist
OutputBaseFilename=Scaner-Setup-{#AppVersion}
SetupIconFile=..\..\build\icon.ico
UninstallDisplayIcon={app}\Scaner.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"

[Files]
Source: "{#AppExe}"; DestDir: "{app}"; DestName: "Scaner.exe"; Flags: ignoreversion

[Icons]
Name: "{group}\Scaner"; Filename: "{app}\Scaner.exe"
Name: "{autodesktop}\Scaner"; Filename: "{app}\Scaner.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\Scaner.exe"; Description: "Launch Scaner"; Flags: nowait postinstall skipifsilent