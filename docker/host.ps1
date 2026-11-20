#Requires -Version 5.1
<#
.SYNOPSIS
  宿主机模式：后端以 Windows 原生 bin\admin.exe 运行，容器只留 aranea-web 与 aranea-egress-proxy。
.DESCRIPTION
  运行台（/fleet）的终端起在后端所在的机器上，所以日常后端要跑在宿主机上才能用。
  方案与取舍：E:\teamwork\docs\aranea-agents\plan\2026-11-19-运行台宿主机部署方案.md

  up        预检 → 生成宿主机配置 → 编译 → 停容器后端 → 起 exe → web 切到宿主机 → 等 /healthz
  build     只生成配置并编译到 bin\host-new\（不影响正在跑的后端；下次 up -SkipBuild 时换上）
  down      停 exe（容器不动）
  rollback  停 exe → docker compose up -d（不带覆盖文件），回到容器后端
  status    进程 / 健康状态
  autostart 注册「登录时自动启动」计划任务（取消：Unregister-ScheduledTask AraneaHostAdmin）
  run       前台运行后端（给计划任务和 up 用，一般不手动调）
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File docker\host.ps1 -Action up
  powershell -ExecutionPolicy Bypass -File docker\host.ps1 -Action up -SkipBuild
  powershell -ExecutionPolicy Bypass -File docker\host.ps1 -Action rollback
#>
param(
  [Parameter(Mandatory = $true)][ValidateSet('up', 'build', 'down', 'rollback', 'status', 'autostart', 'run')][string]$Action,
  [switch]$SkipBuild
)
$ErrorActionPreference = 'Stop'
$repo     = Split-Path $PSScriptRoot -Parent
$exe      = Join-Path $repo 'bin\admin.exe'
$stage    = Join-Path $repo 'bin\host-new'
$conf     = Join-Path $PSScriptRoot 'volumes\host\config.yaml'
$taskName = 'AraneaHostAdmin'
$runArgs  = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$PSCommandPath`" -Action run"
$composeBase = @('compose', '--project-directory', $repo, '-f', (Join-Path $repo 'docker-compose.yaml'))
$composeHost = $composeBase + @('-f', (Join-Path $PSScriptRoot 'docker-compose.host.yaml'))

# 与 dev-up.ps1 同款：PS5.1 + EAP=Stop 下原生命令写 stderr 会抛错，只按退出码判成败。
function Invoke-Native {
  $cmd = $args[0]
  $rest = @()
  if ($args.Count -gt 1) { $rest = $args[1..($args.Count - 1)] }
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    & $cmd @rest 2>&1 | ForEach-Object { Write-Host "$_" }
    return $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $prev
  }
}

function Get-Admin { Get-Process -Name admin -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe } }

function Test-Http($url) {
  try { return ((Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5).StatusCode -eq 200) } catch { return $false }
}

function Wait-Healthy($url, $seconds) {
  $deadline = (Get-Date).AddSeconds($seconds)
  while (-not (Test-Http $url)) {
    if ((Get-Date) -gt $deadline) { return $false }
    Start-Sleep -Seconds 3
  }
  return $true
}

# 工作区 AGENTS.md §8.1-3：启动前校验共享栈，不自己起中间件。
function Test-Infra {
  foreach ($c in 'teamwork-postgres', 'teamwork-redis') {
    $s = cmd /c "docker inspect -f {{.State.Health.Status}} $c 2>nul"
    if ($s -ne 'healthy') { return $false }
  }
  return (Test-Http 'http://127.0.0.1:8222/healthz')
}

function Import-DotEnv {
  # 只取后端要用的键，值不打印。缺 KRATOS_AUTH_SECRET 时后端会悄悄改用本机开发密钥，
  # 已签发的会话全部失效，所以缺了必须硬失败。
  $need = 'KRATOS_AUTH_SECRET', 'ARANEA_TWINOPENAPI_TOKEN', 'TWIN_API_KEY'
  $want = $need + 'ARANEA_ARTIFACT_PUBLIC_BASE'
  foreach ($line in [IO.File]::ReadAllLines((Join-Path $repo '.env'), [Text.Encoding]::UTF8)) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$' -and $want -contains $Matches[1]) {
      $k = $Matches[1]
      $v = $Matches[2]
      if ($v.Length -ge 2 -and (($v[0] -eq '"' -and $v[-1] -eq '"') -or ($v[0] -eq "'" -and $v[-1] -eq "'"))) {
        $v = $v.Substring(1, $v.Length - 2)
      }
      Set-Item -Path "env:$k" -Value $v
    }
  }
  foreach ($k in $need) {
    if (-not [Environment]::GetEnvironmentVariable($k)) { throw ".env 缺少 $k" }
  }
}

function Build-Host {
  & (Join-Path $PSScriptRoot 'gen-config.ps1') -Target host
  if (-not (Select-String -Path $conf -SimpleMatch '127.0.0.1:5436' -Quiet)) {
    throw '生成的配置没有指向共享栈 PG（127.0.0.1:5436）：configs\config.yaml 被改回去了？'
  }
  if ($SkipBuild) { return }
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'PATH 上找不到 go（或加 -SkipBuild）' }
  Push-Location $repo
  try {
    $version = (git describe --tags --always 2>$null); if (-not $version) { $version = 'host' }
    $commit  = (git rev-parse HEAD 2>$null);           if (-not $commit)  { $commit  = 'unknown' }
    $buildDate = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $ldflags = "-X main.Version=$version -X main.Commit=$commit -X main.BuildDate=$buildDate"
    New-Item -ItemType Directory -Force $stage | Out-Null
    Write-Host '编译 admin / fleetctl / acp-shim (windows/amd64)...' -ForegroundColor Cyan
    # 先编到 bin\host-new\：正在跑的 exe 被 Windows 锁着，不能原地覆盖。
    # timetzdata：Windows 没有系统时区库，按名字取时区（Asia/Shanghai 的定时任务）要靠内嵌数据。
    $rc = Invoke-Native go build -tags 'pgvector,timetzdata' -ldflags $ldflags -o "$stage\" .\cmd\admin .\cmd\fleetctl .\cmd\acp-shim-deepseek .\cmd\acp-shim-grok
    if ($rc -ne 0) { throw "go build 失败（$rc）" }
  } finally {
    Pop-Location
  }
}

function Publish-Stage {
  if (-not (Test-Path (Join-Path $stage 'admin.exe'))) { return }
  Get-ChildItem $stage -Filter *.exe | ForEach-Object { Move-Item $_.FullName (Join-Path $repo 'bin') -Force }
  Remove-Item $stage
}

function Stop-Admin {
  # 先停计划任务实例：它是 run 的父进程，不停的话会按失败重启策略把后端再拉起来。
  $t = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($t -and $t.State -eq 'Running') { Stop-ScheduledTask -TaskName $taskName }
  # shortcut: 强杀进程树（Windows 没有 SIGTERM），进行中的请求和运行台终端会断；要优雅停机时改为向进程组发 CTRL_BREAK
  foreach ($p in @(Get-Admin)) { cmd /c "taskkill /F /T /PID $($p.Id) >nul 2>&1" }
  $deadline = (Get-Date).AddSeconds(15)
  while ((Get-Admin) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 300 }
  if (Get-Admin) { throw 'admin.exe 停不掉，请手动结束后重试' }
}

function Start-Backend {
  # 必须用干净的用户环境起后端：运行台终端会继承后端的环境变量，从别的工具
  # （IDE、Claude Code 会话）里直接起，会把它们的变量带进 agent 终端。
  # 计划任务和 WMI 派生的进程拿的是用户登录时的环境，不继承调用方。
  $t = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($t) {
    if ($t.State -eq 'Disabled') { Enable-ScheduledTask -TaskName $taskName | Out-Null }
    Start-ScheduledTask -TaskName $taskName
  } else {
    $r = ([wmiclass]'Win32_Process').Create("powershell.exe $runArgs", $repo)
    if ($r.ReturnValue -ne 0) { throw "启动失败（Win32_Process.Create 返回 $($r.ReturnValue)）" }
  }
}

function Wait-Stack {
  Write-Host '等待 /healthz（含启动迁移，最长 180s）...' -ForegroundColor Cyan
  if (-not (Wait-Healthy 'http://127.0.0.1:8810/healthz' 180)) { throw '后端 /healthz 180s 未就绪' }
  if (-not (Wait-Healthy 'http://127.0.0.1:9402/healthz' 60)) { throw '后端已就绪，但经 aranea-web（9402）访问不通' }
  Write-Host '8810 与 9402 的 /healthz 都已就绪' -ForegroundColor Green
}

switch ($Action) {
  'build' { Build-Host }

  'up' {
    foreach ($t in 'git', 'docker') {
      if (-not (Get-Command $t -ErrorAction SilentlyContinue)) { throw "PATH 上找不到 $t" }
    }
    if (-not (Test-Infra)) {
      throw '共享栈未就绪：teamwork-postgres / teamwork-redis 需 healthy，127.0.0.1:8222/healthz 需可达（先起 E:\teamwork\infra）'
    }
    Import-DotEnv   # 只为在停容器之前发现 .env 缺项；真正给后端用的那次在 run 里
    Build-Host
    if (-not (Test-Path (Join-Path $stage 'admin.exe')) -and -not (Test-Path $exe)) { throw '没有可用的 admin.exe，去掉 -SkipBuild 重试' }
    # 容器期间新写的数据文件补到仓库根 data\（只增不删；robocopy 退出码 <8 为成功）
    robocopy (Join-Path $PSScriptRoot 'volumes\data') (Join-Path $repo 'data') /E /XO /NFL /NDL /NJH /NJS | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "data 同步失败（robocopy $LASTEXITCODE）" }

    # ---- 从这里起后端中断，到 /healthz 就绪为止 ----
    Stop-Admin
    cmd /c 'docker stop aranea-admin >nul 2>&1'
    Publish-Stage
    Start-Backend
    $rc = Invoke-Native docker @composeHost up -d
    if ($rc -ne 0) { throw "compose up 失败（$rc）" }
    try { Wait-Stack } catch {
      Write-Host "启动输出见 $PSScriptRoot\volumes\host\admin.console.log；回退：host.ps1 -Action rollback" -ForegroundColor Yellow
      throw
    }
  }

  'down' { Stop-Admin }

  'rollback' {
    Stop-Admin
    # 不禁用的话下次登录计划任务又会来抢 8810
    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) { Disable-ScheduledTask -TaskName $taskName | Out-Null }
    $rc = Invoke-Native docker @composeBase up -d
    if ($rc -ne 0) { throw "compose up 失败（$rc）" }
    Wait-Stack
  }

  'status' {
    $p = Get-Admin
    if ($p) { "宿主机后端：admin.exe PID $($p.Id)，启动于 $($p.StartTime)" } else { '宿主机后端：未运行' }
    "容器 aranea-admin：$(cmd /c 'docker inspect -f {{.State.Status}} aranea-admin 2>nul')"
    $t = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($t) { "登录自启任务 ${taskName}：$($t.State)" } else { "登录自启任务 ${taskName}：未注册" }
    "8810 /healthz：$(Test-Http 'http://127.0.0.1:8810/healthz')    9402 /healthz：$(Test-Http 'http://127.0.0.1:9402/healthz')"
  }

  'autostart' {
    $me  = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $act = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $runArgs -WorkingDirectory $repo
    $trg = New-ScheduledTaskTrigger -AtLogOn -User $me
    # 交互式登录身份：agent 命令行要用到你的 PATH、登录状态和 git 凭据，不能当系统服务跑
    $prn = New-ScheduledTaskPrincipal -UserId $me -LogonType Interactive -RunLevel Limited
    # 不限时长（默认 72 小时会被杀）；异常退出 1 分钟后重启，最多 3 次
    $set = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -MultipleInstances IgnoreNew -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    Register-ScheduledTask -TaskName $taskName -Action $act -Trigger $trg -Principal $prn -Settings $set -Force | Out-Null
    Write-Host "已注册登录自启任务 $taskName" -ForegroundColor Green
  }

  'run' {
    if (-not (Test-Path $exe))  { throw "找不到 $exe，先执行 -Action up" }
    if (-not (Test-Path $conf)) { throw "找不到 $conf，先执行 -Action up" }
    if (Get-Admin) { throw 'admin.exe 已在运行' }
    # 登录自启时 Docker 可能还没起来：等共享栈就绪（最长 10 分钟）
    $deadline = (Get-Date).AddMinutes(10)
    while (-not (Test-Infra)) {
      if ((Get-Date) -gt $deadline) { throw '共享栈（teamwork-postgres / redis / nats）10 分钟内未就绪' }
      Start-Sleep -Seconds 5
    }
    if (Get-NetTCPConnection -LocalPort 8810 -State Listen -ErrorAction SilentlyContinue) {
      throw '8810 已被占用（容器后端还在跑？用 -Action up 切换）'
    }
    Import-DotEnv
    # 以下与 docker-compose.yaml 的 admin.environment 保持一致。有意的差异只有三处：
    # 宿主机地址用回环、pprof 显式绑回环、文件工具根目录（不设会落到仓库根，能读到 .env 和源码）。
    $env:DEPLOY_ENV = 'dev'
    $env:DAO_VECTOR_PGVECTOR = '1'
    $env:TWIN_GATEWAY_URL = 'http://127.0.0.1:8000'
    $env:GNS3_AGENT_URL = 'http://127.0.0.1:18081'
    $env:ARANEA_OUTBOUND_ALLOW_HOSTS = 'host.docker.internal,127.0.0.1,localhost'
    $env:KRATOS_HTTP_EXTRA_CORS_ORIGINS = 'http://0.0.0.0:9301,http://192.168.0.108:9301,http://192.168.0.108:8810,http://localhost:9301,http://127.0.0.1:9301'
    $env:ARANEA_PPROF_ADDR = '127.0.0.1:8813'
    $env:ARANEA_L0_SNAPSHOT = 'force'
    $env:CODE_EXECUTOR_BACKEND = 'docker'
    $env:CODE_EXECUTOR_DOCKER_IMAGE = 'python:3.11-slim'
    $env:ARANEA_WORKSPACE_ROOT = 'E:\teamwork\temp\aranea-agents\runtime\workspace'
    # 宿主机模式就是单机本地部署：打开制品所在文件夹、资料页「用本机程序打开」和建库选本机文件夹
    $env:FEATURES_LOCAL_REVEAL_ENABLED = 'true'
    New-Item -ItemType Directory -Force $env:ARANEA_WORKSPACE_ROOT | Out-Null
    # 工作目录必须是仓库根：internal\scenario、configs\workflows、data\ 都按相对路径找。
    # 相对路径不含空格，交给 cmd 做重定向可以不加引号。
    Set-Location $repo
    cmd /c 'bin\admin.exe -conf docker\volumes\host\config.yaml > docker\volumes\host\admin.console.log 2>&1'
    exit $LASTEXITCODE
  }
}
