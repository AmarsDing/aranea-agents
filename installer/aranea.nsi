; Aranea-Agents NSIS installer
; NOTE: Keep user-visible strings in ASCII/English to avoid mojibake on finish page
; when the .nsi file encoding differs from the builder code page.
; Usage: makensis /DVERSION=0.1.32 /DSTAGING_DIR=build\staging /DOUT_DIR=release aranea.nsi

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

VIProductVersion "0.1.32.0"
VIAddVersionKey "ProductName" "Aranea-Agents"
VIAddVersionKey "CompanyName" "AmarsDing"
VIAddVersionKey "LegalCopyright" "Copyright (C) 2026 AmarsDing"
VIAddVersionKey "FileDescription" "Aranea-Agents Multi-Agent Platform"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

!define MUI_ABORTWARNING
!define MUI_ICON "${STAGING_DIR}\frontend\icon.ico"
!define MUI_UNICON "${STAGING_DIR}\frontend\icon.ico"

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

  File /r "${STAGING_DIR}\"

  IfFileExists "$INSTDIR\AraneaLauncher.exe" launcher_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: AraneaLauncher.exe missing. Please re-download the package."
    Abort
  launcher_ok:
  IfFileExists "$INSTDIR\aranea-server.exe" server_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: aranea-server.exe missing."
    Abort
  server_ok:
  IfFileExists "$INSTDIR\frontend\AraneaAgents.exe" frontend_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: frontend\AraneaAgents.exe missing."
    Abort
  frontend_ok:
  IfFileExists "$INSTDIR\internal\scenario\system\prompts\IDENTITY.md" scenario_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: internal\scenario prompts missing. Please re-download the package."
    Abort
  scenario_ok:
  IfFileExists "$INSTDIR\data\model-catalog\current.json" catalog_ok
    MessageBox MB_OK|MB_ICONSTOP "Install failed: data\model-catalog missing. Please re-download the package."
    Abort
  catalog_ok:

  ; --- WebView2 Runtime check (required by the Tauri desktop app) ---
  ; Evergreen runtime client GUID; presence of "pv" value means installed.
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKCU "SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    DetailPrint "WebView2 Runtime not found; downloading bootstrapper..."
    nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Invoke-WebRequest -Uri https://go.microsoft.com/fwlink/p/?LinkId=2124703 -OutFile $TEMP\MicrosoftEdgeWebview2Setup.exe -UseBasicParsing"'
    Pop $1
    ${If} $1 == 0
      DetailPrint "Installing WebView2 Runtime (silent)..."
      nsExec::ExecToLog '"$TEMP\MicrosoftEdgeWebview2Setup.exe" /silent /install'
      Pop $1
      ${If} $1 != 0
        MessageBox MB_OK|MB_ICONEXCLAMATION "WebView2 Runtime installation failed (exit $1). The desktop app requires WebView2 - please install it manually from https://developer.microsoft.com/microsoft-edge/webview2/"
      ${EndIf}
    ${Else}
      MessageBox MB_OK|MB_ICONEXCLAMATION "WebView2 bootstrapper download failed. The desktop app requires WebView2 - please install it manually from https://developer.microsoft.com/microsoft-edge/webview2/"
    ${EndIf}
  ${Else}
    DetailPrint "WebView2 Runtime found (version $0)."
  ${EndIf}

  CreateDirectory "$SMPROGRAMS\Aranea-Agents"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "" "$INSTDIR\frontend\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Environment Check.lnk" "$INSTDIR\AraneaLauncher.exe" "-check" "$INSTDIR\frontend\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Stop Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "-stop" "$INSTDIR\frontend\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Uninstall.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\frontend\icon.ico"

  ; Do NOT run AraneaLauncher during install (can hang NSIS).

  CreateShortcut "$DESKTOP\Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "" "$INSTDIR\frontend\icon.ico"

  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayName" "Aranea-Agents"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayIcon" "$INSTDIR\frontend\icon.ico"
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
