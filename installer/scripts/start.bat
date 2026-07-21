@echo off
REM Deprecated debug entry. Always hand off to AraneaLauncher.exe with no console work.
cd /d "%~dp0"
if not exist "%~dp0AraneaLauncher.exe" (
  mshta "javascript:alert('Missing AraneaLauncher.exe. Please reinstall Aranea-Agents.');close()"
  exit /b 1
)
start "" "%~dp0AraneaLauncher.exe" %*
exit /b 0
