; Aranea-Agents NSIS installer
; NOTE: Keep user-visible strings in ASCII/English to avoid mojibake on finish page
; when the .nsi file encoding differs from the builder code page.
; Usage: makensis /DVERSION=0.1.30 /DSTAGING_DIR=build\staging /DOUT_DIR=release aranea.nsi

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef STAGING_DIR
  !define STAGING_DIR "build\staging"
!endif
!ifndef OUT_DIR
  !define OUT_DIR "release"
!endif

Name "Aranea-Agents"
OutFile "${OUT_DIR}\AraneaAgents-${VERSION}-win-x64.exe"
Unicode True
ShowInstDetails show
ShowUnInstDetails show

InstallDir "$LOCALAPPDATA\AraneaAgents"
RequestExecutionLevel user

VIProductVersion "0.1.30.0"
VIAddVersionKey "ProductName" "Aranea-Agents"
VIAddVersionKey "CompanyName" "AmarsDing"
VIAddVersionKey "LegalCopyright" "Copyright (C) 2026 AmarsDing"
VIAddVersionKey "FileDescription" "Aranea-Agents Multi-Agent Platform"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

!define MUI_ABORTWARNING
!define MUI_ICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"
!define MUI_UNICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

; ASCII-only finish strings (prevents garbled Chinese on some Windows locales)
!define MUI_FINISHPAGE_RUN "$INSTDIR\AraneaLauncher.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Start Aranea-Agents"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.md"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Open README"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; English first as default language to avoid garbled SimpChinese UI from encoding mismatch
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

Section "MainSection" SecMain
  SetOutPath "$INSTDIR"

  DetailPrint "Stopping previous Aranea processes..."
  nsExec::ExecToLog 'taskkill /IM AraneaAgents.exe /F /T'
  nsExec::ExecToLog 'taskkill /IM aranea-server.exe /F /T'
  nsExec::ExecToLog 'taskkill /IM AraneaLauncher.exe /F /T'
  Sleep 800

  File /nonfatal /r "${STAGING_DIR}\*.*"

  IfFileExists "$INSTDIR\AraneaLauncher.exe" launcher_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: AraneaLauncher.exe missing. Please re-download the package."
    Abort
  launcher_ok:
  IfFileExists "$INSTDIR\aranea-server.exe" server_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: aranea-server.exe missing."
    Abort
  server_ok:
  IfFileExists "$INSTDIR\frontend\AraneaAgents.exe" electron_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: frontend\AraneaAgents.exe missing."
    Abort
  electron_ok:

  CreateDirectory "$SMPROGRAMS\Aranea-Agents"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Environment Check.lnk" "$INSTDIR\AraneaLauncher.exe" "-check" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Stop Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "-stop" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Uninstall.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"

  ; Do NOT run AraneaLauncher during install (can hang NSIS).

  CreateShortcut "$DESKTOP\Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"

  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayName" "Aranea-Agents"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayIcon" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "Publisher" "AmarsDing"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "NoRepair" 1

  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "EstimatedSize" "$0"

  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$INSTDIR\logs"
  SetOutPath "$INSTDIR"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'taskkill /IM AraneaAgents.exe /F /T'
  nsExec::ExecToLog 'taskkill /IM aranea-server.exe /F /T'
  nsExec::ExecToLog 'taskkill /IM AraneaLauncher.exe /F /T'
  nsExec::ExecToLog 'taskkill /IM redis-server.exe /F /T'
  Sleep 1000
  IfFileExists "$INSTDIR\postgres\bin\pg_ctl.exe" 0 skip_pg_stop
    nsExec::ExecToLog '"$INSTDIR\postgres\bin\pg_ctl.exe" stop -D "$INSTDIR\postgres\data" -m fast'
  skip_pg_stop:

  Delete "$DESKTOP\Aranea-Agents.lnk"
  RMDir /r "$SMPROGRAMS\Aranea-Agents"

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents"

  Delete "$INSTDIR\aranea-server.exe"
  Delete "$INSTDIR\AraneaLauncher.exe"
  Delete "$INSTDIR\start.bat"
  Delete "$INSTDIR\stop.bat"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\configs\config.yaml"
  RMDir "$INSTDIR\configs"

  RMDir /r "$INSTDIR\frontend"
  RMDir /r "$INSTDIR\redis"
  RMDir /r "$INSTDIR\logs"

  RMDir /r "$INSTDIR\postgres\bin"
  RMDir /r "$INSTDIR\postgres\lib"
  RMDir /r "$INSTDIR\postgres\share"

  IfFileExists "$INSTDIR\postgres\data" 0 skip_data
    MessageBox MB_YESNO|MB_ICONQUESTION "Delete database data? This cannot be undone." IDNO skip_data
    RMDir /r "$INSTDIR\postgres\data"
  skip_data:

  RMDir "$INSTDIR\postgres"
  RMDir "$INSTDIR"

  SetAutoClose True
SectionEnd
