#Requires -Version 5.1
<#
.SYNOPSIS
  Register Dockerized Aranea into TwinMonitor monitoring (idempotent).
.DESCRIPTION
  1) Device chain: monitor_assets + monitor_devices + icmp protocol binding
     + collection_tasks + collector hot reload (device online status via ICMP ping).
  2) Line monitors: HTTP GET /healthz + TCP gRPC/WS port probes
     (linemonitor probe engine hot-refreshes on config upsert).
  Prereq: TwinMonitor gateway(:8000) online; aranea-admin container publishing 8810/9910/8812.
  NOTE: keep this file UTF-8 *with BOM*; PS5.1 decodes BOM-less .ps1 as GBK.
#>
param(
  [string]$GatewayBase = "http://127.0.0.1:8000",
  [string]$Username = "admin",
  [string]$Password = "123456",
  [string]$AraneaHost = "127.0.0.1",
  [int]$HttpPort = 8810,
  [int]$GrpcPort = 9910,
  [int]$WsPort = 8812,
  [string]$AssetCode = "ARANEA-DOCKER-01",
  [string]$AssetName = "Aranea-Docker-Host"
)

$ErrorActionPreference = "Stop"
$script:token = ""

function Invoke-Api {
  param(
    [Parameter(Mandatory=$true)][string]$Method,
    [Parameter(Mandatory=$true)][string]$Path,
    [object]$Body = $null,
    [string]$Query = ""
  )
  $uri = "$GatewayBase$Path"
  if ($Query -ne "") { $uri = "$uri`?$Query" }
  $headers = @{}
  if ($script:token -ne "") { $headers["Authorization"] = "Bearer $($script:token)" }
  $p = @{ Uri = $uri; Method = $Method; Headers = $headers; TimeoutSec = 15; ContentType = "application/json; charset=utf-8" }
  if ($null -ne $Body) { $p["Body"] = ($Body | ConvertTo-Json -Depth 10 -Compress) }
  try {
    return Invoke-RestMethod @p
  } catch {
    $detail = ""
    if ($_.ErrorDetails) { $detail = $_.ErrorDetails.Message }
    throw "API $Method $Path failed: $($_.Exception.Message) $detail"
  }
}

# ---------- 0. login ----------
$login = Invoke-Api -Method Post -Path "/api/v1/monitor/auth/login" -Body @{ grant_type = "password"; username = $Username; password = $Password }
$script:token = $login.access_token
if (-not $script:token) { throw "login returned empty token" }
Write-Host "[ok] login gateway $GatewayBase"

# ---------- 1. asset ----------
$assetId = 0
$assets = Invoke-Api -Method Get -Path "/api/v1/monitor/monitor-assets" -Query ("query=" + [uri]::EscapeDataString('{"asset_code":"' + $AssetCode + '"}'))
if ($assets.items -and $assets.items.Count -gt 0) {
  $assetId = $assets.items[0].id
  Write-Host "[skip] asset exists id=$assetId code=$AssetCode"
} else {
  $created = Invoke-Api -Method Post -Path "/api/v1/monitor/monitor-assets" -Body @{ data = @{
    assetCode    = $AssetCode
    name         = $AssetName
    isDevice     = 1
    needsMonitor = 1
    status       = 1
    location     = "local-docker"
    remark       = "Aranea docker compose node (admin 8810/9910/8812)"
  } }
  $assetId = $created.id
  Write-Host "[ok] asset created id=$assetId code=$AssetCode"
}

# ---------- 2. monitor device ----------
$deviceId = 0
$deviceMonitorStatus = -1
$devs = Invoke-Api -Method Get -Path "/api/v1/monitor/monitor-devices" -Query "noPaging=true"
if ($devs.items) { foreach ($d in $devs.items) { if ($d.assetId -eq $assetId) { $deviceId = $d.id; $deviceMonitorStatus = [int]$d.monitorStatus } } }
if ($deviceId -gt 0) {
  Write-Host "[skip] monitor device exists id=$deviceId"
} else {
  Invoke-Api -Method Post -Path "/api/v1/monitor/monitor-devices" -Body @{ data = @{
    assetId       = $assetId
    monitorStatus = 1   # 1 = monitor + alarm
    internal      = 30  # default collect interval seconds
  } } | Out-Null
  $devs = Invoke-Api -Method Get -Path "/api/v1/monitor/monitor-devices" -Query "noPaging=true"
  foreach ($d in $devs.items) { if ($d.assetId -eq $assetId) { $deviceId = $d.id; $deviceMonitorStatus = [int]$d.monitorStatus } }
  if ($deviceId -eq 0) { throw "monitor device not found after create (asset=$assetId)" }
  Write-Host "[ok] monitor device created id=$deviceId"
}
# ensure monitor+alarm (create may default monitor_status=0 when field dropped by transcoding)
if ($deviceMonitorStatus -ne 1) {
  Invoke-Api -Method Put -Path "/api/v1/monitor/monitor-devices/$deviceId" -Body @{
    data       = @{ id = $deviceId; monitorStatus = 1 }
    updateMask = "monitorStatus"
  } | Out-Null
  Write-Host "[ok] monitor_status corrected to 1 (was $deviceMonitorStatus) device=$deviceId"
}

# ---------- 3. icmp protocol binding ----------
$bindingId = 0
$bindings = Invoke-Api -Method Get -Path "/api/v1/monitor/devices/$deviceId/protocols"
if ($bindings.items) {
  foreach ($b in $bindings.items) { if ($b.protocolType -eq "icmp") { $bindingId = $b.id } }
}
if ($bindingId -gt 0) {
  Write-Host "[skip] icmp binding exists id=$bindingId"
} else {
  Invoke-Api -Method Post -Path "/api/v1/monitor/devices/$deviceId/protocols" -Body @{
    deviceId     = $deviceId
    protocolType = "icmp"
    isEnabled    = 1
    config       = @{
      host        = $AraneaHost
      count       = 4
      interval    = 200
      timeout     = 3000
      packet_size = 56
      ttl         = 64
      metrics     = @("rtt_min", "rtt_avg", "rtt_max", "packet_loss")
    }
  } | Out-Null
  $bindings = Invoke-Api -Method Get -Path "/api/v1/monitor/devices/$deviceId/protocols"
  foreach ($b in $bindings.items) { if ($b.protocolType -eq "icmp") { $bindingId = $b.id } }
  Write-Host "[ok] icmp binding created id=$bindingId"
}

# ---------- 4. collection task ----------
$taskExists = $false
$tasks = Invoke-Api -Method Get -Path "/api/v1/monitor/collector/tasks" -Query "noPaging=true"
if ($tasks.items) { foreach ($t in $tasks.items) { if ($t.deviceId -eq $deviceId -and $t.protocolType -eq "icmp") { $taskExists = $true } } }
if ($taskExists) {
  Write-Host "[skip] collection task exists (device=$deviceId proto=icmp)"
} else {
  Invoke-Api -Method Post -Path "/api/v1/monitor/collector/tasks" -Body @{ data = @{
    deviceId           = $deviceId
    protocolType       = "icmp"
    protocolBindingId  = $bindingId
    taskName           = "Aranea-Docker-ICMP"
    priority           = 2
    collectionInterval = 30
    timeout            = 10
    retryCount         = 1
    retryInterval      = 5
    status             = 1
    offlineThreshold   = 3
  } } | Out-Null
  Write-Host "[ok] collection task created (device=$deviceId binding=$bindingId)"
}

# ---------- 5. collector hot reload ----------
Invoke-Api -Method Post -Path "/api/v1/monitor/collector/reload" -Body @{} | Out-Null
Write-Host "[ok] collector reload triggered"

# ---------- 6. line monitors (HTTP healthz + TCP ports) ----------
function Ensure-Line {
  param(
    [string]$Name,
    [string]$ProbeProtocol,
    [string]$ProbeParams,
    [string]$TargetIp,
    [string]$Desc
  )
  $lineId = 0
  # NOTE: ListLines "status" defaults to 0 (disabled only); must pass status=-1 for all.
  # keyword fuzzy match is unreliable for hyphenated names; pull all + exact-match client-side.
  $q = "status=-1&pageSize=500"
  $lines = Invoke-Api -Method Get -Path "/api/v1/monitor/linemonitor/lines" -Query $q
  if ($lines.items) { foreach ($l in $lines.items) { if ($l.name -eq $Name) { $lineId = $l.id } } }
  if ($lineId -eq 0) {
    Invoke-Api -Method Post -Path "/api/v1/monitor/linemonitor/lines" -Body @{ data = @{
      name        = $Name
      lineType    = "ethernet"
      targetIp    = $TargetIp
      status      = 1
      description = $Desc
    } } | Out-Null
    $lines = Invoke-Api -Method Get -Path "/api/v1/monitor/linemonitor/lines" -Query $q
    foreach ($l in $lines.items) { if ($l.name -eq $Name) { $lineId = $l.id } }
    if ($lineId -eq 0) { throw "line not found after create: $Name" }
    Write-Host "[ok] line created id=$lineId name=$Name"
  } else {
    Write-Host "[skip] line exists id=$lineId name=$Name"
  }
  Invoke-Api -Method Put -Path "/api/v1/monitor/linemonitor/lines/$lineId/monitor-config" -Body @{
    id   = $lineId
    data = @{
      lineId          = $lineId
      monitorInterval = 30
      probeProtocol   = $ProbeProtocol
      probeParams     = $ProbeParams
      timeout         = 3000
      retryCount      = 1
      offlineThreshold = 2
      status          = 1
    }
  } | Out-Null
  Write-Host "[ok] monitor-config upserted line=$lineId proto=$ProbeProtocol"
  return $lineId
}

$httpParams = (@{ http = @{ url = "http://${AraneaHost}:$HttpPort/healthz"; method = "GET"; expected_status = 200; body_regex = '"status"\s*:\s*"ok"' } } | ConvertTo-Json -Depth 5 -Compress)
$l1 = Ensure-Line -Name "ARANEA-HTTP-HEALTH" -ProbeProtocol "HTTP" -ProbeParams $httpParams -TargetIp $AraneaHost -Desc "Aranea admin HTTP health (docker)"

$tcpGrpc = (@{ tcp = @{ port = $GrpcPort; connect_timeout = 3000 } } | ConvertTo-Json -Compress)
$l2 = Ensure-Line -Name "ARANEA-TCP-GRPC" -ProbeProtocol "TCP" -ProbeParams $tcpGrpc -TargetIp $AraneaHost -Desc "Aranea gRPC 9910 port (docker)"

$tcpWs = (@{ tcp = @{ port = $WsPort; connect_timeout = 3000 } } | ConvertTo-Json -Compress)
$l3 = Ensure-Line -Name "ARANEA-TCP-WS" -ProbeProtocol "TCP" -ProbeParams $tcpWs -TargetIp $AraneaHost -Desc "Aranea WS 8812 port (docker)"

# ---------- 7. summary ----------
$dash = Invoke-Api -Method Get -Path "/api/v1/monitor/linemonitor/dashboard"
Write-Host ""
Write-Host "==== registration summary ===="
Write-Host "asset=$assetId device=$deviceId binding=$bindingId lines=$l1,$l2,$l3"
Write-Host "line dashboard: total=$($dash.total) online=$($dash.online) offline=$($dash.offline) degraded=$($dash.degraded)"
