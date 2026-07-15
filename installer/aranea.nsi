; ─── Aranea-Agents NSIS 安装脚本 ──────────────────────────────
; 用法：makensis /DVERSION=v1.0.0 /DSTAGING_DIR=build\staging /DOUT_DIR=release aranea.nsi
;
; UX 原则：
;   - 默认安装到 %LOCALAPPDATA%\AraneaAgents（无需管理员 / 无 UAC）
;   - 桌面快捷方式指向 AraneaLauncher.exe（无黑框控制台）
;   - start.bat 仅作调试备用入口

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

; 用户目录安装：普通用户可写 postgres\data / logs，无需管理员提权
InstallDir "$LOCALAPPDATA\AraneaAgents"
RequestExecutionLevel user

; VIProductVersion 必须为 X.X.X.X；从 VERSION 粗解析，失败则用 0.1.0.0
!define /date BUILD_YEAR "%Y"
VIProductVersion "0.1.24.0"
VIAddVersionKey "ProductName" "Aranea-Agents"
VIAddVersionKey "CompanyName" "AmarsDing"
VIAddVersionKey "LegalCopyright" "Copyright (C) 2026 AmarsDing"
VIAddVersionKey "FileDescription" "Aranea-Agents - Enterprise Multi-Agent Orchestration Platform"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

!define MUI_ABORTWARNING
!define MUI_ICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"
!define MUI_UNICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

; 完成后直接启动静默 launcher（无黑框）
!define MUI_FINISHPAGE_RUN "$INSTDIR\AraneaLauncher.exe"
!define MUI_FINISHPAGE_RUN_TEXT "启动 Aranea-Agents"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.md"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "查看 README"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Section "MainSection" SecMain
  SetOutPath "$INSTDIR"

  ; 安装前先尝试停止旧实例，避免覆盖运行中的 exe
  IfFileExists "$INSTDIR\AraneaLauncher.exe" 0 +2
    nsExec::Exec '"$INSTDIR\AraneaLauncher.exe" -stop'
  Sleep 1500

  File /nonfatal /r "${STAGING_DIR}\*.*"

  CreateDirectory "$SMPROGRAMS\Aranea-Agents"
  ; 主入口：静默 launcher
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\停止 Aranea-Agents.lnk" "$INSTDIR\AraneaLauncher.exe" "-stop" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\启动（调试控制台）.lnk" "$INSTDIR\start.bat" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\卸载.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"

  ; 桌面快捷方式 → 静默 launcher（不再指向 start.bat）
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
  IfFileExists "$INSTDIR\AraneaLauncher.exe" 0 +3
    nsExec::Exec '"$INSTDIR\AraneaLauncher.exe" -stop'
    Sleep 2000
  IfFileExists "$INSTDIR\stop.bat" 0 +2
    nsExec::Exec '"$INSTDIR\stop.bat"'

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
    MessageBox MB_YESNO|MB_ICONQUESTION "是否删除数据库数据？这将删除所有 Agent 数据，且不可恢复！" IDNO skip_data
    RMDir /r "$INSTDIR\postgres\data"
  skip_data:

  RMDir "$INSTDIR\postgres"
  RMDir "$INSTDIR"

  SetAutoClose True
SectionEnd
