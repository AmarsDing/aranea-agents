; ─── Aranea-Agents NSIS 安装脚本 ──────────────────────────────
; 用法：makensis /DVERSION=v1.0.0 /DSTAGING_DIR=build\staging /DOUT_DIR=release aranea.nsi

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

; ─── 版本信息（通过命令行参数传入）────────────────────────────
!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef STAGING_DIR
  !define STAGING_DIR "build\staging"
!endif
!ifndef OUT_DIR
  !define OUT_DIR "release"
!endif

; ─── 基本信息 ──────────────────────────────────────────────────
Name "Aranea-Agents"
OutFile "${OUT_DIR}\AraneaAgents-${VERSION}-win-x64.exe"
Unicode True
ShowInstDetails show
ShowUnInstDetails show

; 安装目录（默认 Program Files）
InstallDir "$PROGRAMFILES64\AraneaAgents"

; 请求管理员权限（需要写入 Program Files 和注册服务）
RequestExecutionLevel admin

; ─── 版本信息资源 ─────────────────────────────────────────────
VIProductVersion "1.0.0.0"
VIAddVersionKey "ProductName" "Aranea-Agents"
VIAddVersionKey "CompanyName" "AmarsDing"
VIAddVersionKey "LegalCopyright" "Copyright (C) 2026 AmarsDing"
VIAddVersionKey "FileDescription" "Aranea-Agents - Enterprise Multi-Agent Orchestration Platform"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

; ─── MUI 界面配置 ──────────────────────────────────────────────
!define MUI_ABORTWARNING
!define MUI_ICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"
!define MUI_UNICON "${STAGING_DIR}\frontend\resources\app\icons\icon.ico"

; Welcome 页面
!insertmacro MUI_PAGE_WELCOME

; License 页面（如果有的话）
; !insertmacro MUI_PAGE_LICENSE "LICENSE.txt"

; 目录选择页面
!insertmacro MUI_PAGE_DIRECTORY

; 安装进度页面
!insertmacro MUI_PAGE_INSTFILES

; 完成页面
!define MUI_FINISHPAGE_RUN "$INSTDIR\start.bat"
!define MUI_FINISHPAGE_RUN_TEXT "启动 Aranea-Agents"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.md"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "查看 README"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_FINISH

; 卸载页面
!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; 语言
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ─── 安装区段 ──────────────────────────────────────────────────
Section "MainSection" SecMain
  SetOutPath "$INSTDIR"

  ; 清理可能存在的旧文件（保留 postgres\data 用户数据）
  ; 注意：不删除 postgres\data 目录，保护用户数据

  ; 复制所有文件
  File /nonfatal /r "${STAGING_DIR}\*.*"

  ; 创建开始菜单快捷方式
  CreateDirectory "$SMPROGRAMS\Aranea-Agents"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\Aranea-Agents.lnk" "$INSTDIR\start.bat" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\停止 Aranea-Agents.lnk" "$INSTDIR\stop.bat" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  CreateShortcut "$SMPROGRAMS\Aranea-Agents\卸载.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"

  ; 创建桌面快捷方式
  CreateShortcut "$DESKTOP\Aranea-Agents.lnk" "$INSTDIR\start.bat" "" "$INSTDIR\frontend\resources\app\icons\icon.ico"

  ; 写入注册表卸载信息
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayName" "Aranea-Agents"
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayVersion" "${VERSION}"
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "DisplayIcon" "$INSTDIR\frontend\resources\app\icons\icon.ico"
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "Publisher" "AmarsDing"
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "InstallLocation" "$INSTDIR"
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "NoModify" 1
  WriteRegDWORD SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "NoRepair" 1

  ; 估算安装大小写入注册表
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents" "EstimatedSize" "$0"

  ; 创建卸载程序
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; 给 logs/ 和 postgres/ 目录授予 Users 组写权限
  ; 这样普通用户运行 start.bat 时 PostgreSQL initdb 可创建 data/ 子目录
  ; (双保险：start.bat 也会自我提权；ACL 让 UAC 弹窗可避免)
  CreateDirectory "$INSTDIR\logs"
  nsExec::ExecToLog 'icacls "$INSTDIR\logs" /grant Users:(OI)(CI)F /T'
  nsExec::ExecToLog 'icacls "$INSTDIR\postgres" /grant Users:(OI)(CI)F /T'

  ; 标记安装成功
  SetOutPath "$INSTDIR"
SectionEnd

; ─── 卸载区段 ──────────────────────────────────────────────────
Section "Uninstall"
  ; 先停止运行中的服务
  IfFileExists "$INSTDIR\stop.bat" 0 +2
    nsExec::Exec '"$INSTDIR\stop.bat"'

  Sleep 2000

  ; 删除快捷方式
  Delete "$DESKTOP\Aranea-Agents.lnk"
  RMDir /r "$SMPROGRAMS\Aranea-Agents"

  ; 删除注册表项
  DeleteRegKey SHCTX "Software\Microsoft\Windows\CurrentVersion\Uninstall\AraneaAgents"

  ; 删除安装文件（保留 postgres\data 用户数据）
  Delete "$INSTDIR\aranea-server.exe"
  Delete "$INSTDIR\start.bat"
  Delete "$INSTDIR\stop.bat"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\configs\config.yaml"
  RMDir "$INSTDIR\configs"

  RMDir /r "$INSTDIR\frontend"
  RMDir /r "$INSTDIR\redis"
  RMDir /r "$INSTDIR\logs"

  ; PostgreSQL: 删除 bin/lib/share 但保留 data/
  RMDir /r "$INSTDIR\postgres\bin"
  RMDir /r "$INSTDIR\postgres\lib"
  RMDir /r "$INSTDIR\postgres\share"

  ; 询问是否删除数据库
  IfFileExists "$INSTDIR\postgres\data" 0 skip_data
    MessageBox MB_YESNO|MB_ICONQUESTION "是否删除数据库数据？这将删除所有 Agent 数据，且不可恢复！" IDNO skip_data
    RMDir /r "$INSTDIR\postgres\data"
  skip_data:

  RMDir "$INSTDIR\postgres"
  RMDir "$INSTDIR"

  SetAutoClose True
SectionEnd
