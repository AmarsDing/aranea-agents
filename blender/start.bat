@echo off
rem ============================================================
rem  Blender 3D 资产预览 - 一键启动脚本
rem  GLB/FBX/manifest 需 HTTP 协议加载，浏览器 file:// 无法打开
rem  本脚本以 blender/ 为根目录启动本地 HTTP 服务并打开导航页
rem ============================================================
cd /d %~dp0
set PORT=8930

where python >nul 2>nul
if %errorlevel%==0 goto :run
where py >nul 2>nul
if %errorlevel%==0 (set PYTHON=py) else (
    echo [错误] 未找到 python，请先安装 Python 或将其加入 PATH
    pause
    exit /b 1
)
goto :runpy

:run
set PYTHON=python
:runpy
echo 启动预览服务: http://localhost:%PORT%/
start "" "http://localhost:%PORT%/"
%PYTHON% -m http.server %PORT% --bind 127.0.0.1
