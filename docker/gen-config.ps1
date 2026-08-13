#Requires -Version 5.1
<#
.SYNOPSIS
  生成 Aranea Docker overlay 配置：从源仓库现行配置复制并做确定性替换。
.DESCRIPTION
  源：configs\config.yaml（当前运行真理，PORT-PLAN 治理后口径）
  目标：docker\config\config.yaml（compose 挂载到容器 /app/conf）
  替换规则（全部为字面量替换，无正则歧义）：
    1) 监听端口：8800→8810 / 9900→9910 / 8802→8812（源仓库 dev 端口 → PORT-PLAN 部署版）
    2) 中间件：127.0.0.1:5432→postgres:5432；127.0.0.1:6379→redis:6379（compose 服务名）
    3) 路径：./bin/logs → /app/logs（每容器卷挂载到宿主机 docker/volumes/logs）
    4) self_improvement.sandbox.repo_root：f:\aranea-agents → 空（容器内无仓库，降级为进程工作目录）
  幂等：每次从源配置全量重新生成。
  注意：DB 系统设置（embedding base_url、MCP/channel 回调等 localhost 出站地址）
        不在本文件作用域，切换后需在 UI/DB 改指 host.docker.internal（见 docker/migrate-data.ps1 尾注）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File gen-config.ps1
#>
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$src  = Join-Path $repo 'configs\config.yaml'
$dstDir = Join-Path $PSScriptRoot 'config'
$dst  = Join-Path $dstDir 'config.yaml'

New-Item -ItemType Directory -Force $dstDir | Out-Null

$t = [IO.File]::ReadAllText($src, [Text.Encoding]::UTF8)

# 1) 监听端口（dev → 部署版）
$t = $t.Replace('0.0.0.0:8800', '0.0.0.0:8810')
$t = $t.Replace('0.0.0.0:9900', '0.0.0.0:9910')
$t = $t.Replace('0.0.0.0:8802', '0.0.0.0:8812')
# 2) 中间件 → compose 服务名
$t = $t.Replace('127.0.0.1:5432', 'postgres:5432').Replace('localhost:5432', 'postgres:5432')
$t = $t.Replace('127.0.0.1:6379', 'redis:6379').Replace('localhost:6379', 'redis:6379')
# 3) 日志路径
$t = $t.Replace('"./bin/logs"', '"/app/logs"').Replace('./bin/logs', '/app/logs')
# 4) self_improvement 沙箱仓库根（容器内无宿主机路径）
$t = $t.Replace('"f:\\aranea-agents"', '""')

[IO.File]::WriteAllText($dst, $t, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "overlay 配置已生成: $dst" -ForegroundColor Green

# 残留检查：输出仍含 localhost/127.0.0.1 的行（应只剩注释或合法回环）
Write-Host '== 残留 localhost/127.0.0.1 检查 ==' -ForegroundColor Cyan
$hits = Select-String -Path $dst -Pattern 'localhost|127\.0\.0\.1' |
  Where-Object { $_.Line -notmatch '^\s*#' }
if ($hits) { $hits | ForEach-Object { Write-Host ("  L{0}: {1}" -f $_.LineNumber, $_.Line.Trim()) -ForegroundColor Yellow } }
else { Write-Host '  无残留' -ForegroundColor Green }
