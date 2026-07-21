@echo off
set KRATOS_AUTH_SECRET=aranea-portable-dev-secret-32chars!!
set DEPLOY_ENV=dev
set KRATOS_HTTP_AUTH_DISABLED=1
F:\aranea-agents\admin.exe -conf F:\aranea-agents\configs\config.yaml > F:\aranea-agents\_temp\admin_stdout.log 2> F:\aranea-agents\_temp\admin_stderr.log
echo Exit code: %ERRORLEVEL%
