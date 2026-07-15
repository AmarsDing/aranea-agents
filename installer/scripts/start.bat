@echo off
REM Deprecated debug entry. Always hand off to AraneaLauncher.exe with no console work.
cd /d "%~dp0"
if not exist "%~dp0AraneaLauncher.exe" (
  mshta "javascript:alert('缺少 AraneaLauncher.exe，请重新安装 Aranea-Agents');close()"
  exit /b 1
)
REM start with empty title — no lingering console work after this line
start "" "%~dp0AraneaLauncher.exe" %*
exit /b 0
