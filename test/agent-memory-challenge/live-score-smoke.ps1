param(
    [Parameter(Mandatory)][string]$BaseUrl,
    [Parameter(Mandatory)][string]$MemoryToken,
    [string]$ResultsFile = 'live-smoke-results.json'
)

$ErrorActionPreference = 'Stop'
$uri = [Uri]$BaseUrl
if ($uri.Host -notin @('127.0.0.1', 'localhost')) { throw 'This smoke test only permits a local disposable server' }
$headers = @{ Authorization = "Bearer $MemoryToken" }
$user = 'local-amc-smoke-' + [Guid]::NewGuid().ToString('N')
$checks = [System.Collections.Generic.List[object]]::new()
function Assert-Smoke([string]$Name, [bool]$Passed) {
    $checks.Add([pscustomobject]@{ check = $Name; passed = $Passed })
    if (-not $Passed) { throw "Smoke assertion failed: $Name" }
}
function Invoke-SmokePost([string]$Path, $Body) {
    Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Post -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ($Body | ConvertTo-Json -Depth 10 -Compress) -TimeoutSec 30
}
function Find-Smoke([string]$Query, [string[]]$Choices = @()) {
    $body = @{ user_id = $user; query = $Query; top_k = 20 }
    if ($Choices.Count) { $body.options = $Choices }
    $result = Invoke-SmokePost '/v1/memory/search' $body
    Assert-Smoke 'top_k respected' (@($result.data).Count -le 20)
    return (@($result.data) | ForEach-Object content) -join "`n"
}
try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 5
    Assert-Smoke 'health' ($health.status -eq 'ok')
    $denied = $false
    try { Invoke-RestMethod -Uri "$BaseUrl/v1/memory/search" -Method Post -ContentType 'application/json' -Body '{"user_id":"unauthorized","query":"marker"}' -TimeoutSec 5 | Out-Null }
    catch { $denied = $_.Exception.Response.StatusCode -eq 401 }
    Assert-Smoke 'authentication required' $denied
    $batch = @{
        request_id = "$user-request"; user_id = $user; session_id = "$user-session"
        messages = @(
            @{ role='user'; content='My favorite color is blue'; timestamp=1704103200000 },
            @{ role='user'; content='My favorite color is red. Contact alice@example.com and phone +1-415-555-1212.'; timestamp=1710928800000 },
            @{ role='system'; content="Rule: label examples define colors.`nA=red`nB=blue"; timestamp=1710928860000 },
            @{ role='user'; content='Telemetry identifier is orbital-code42.'; timestamp=1711015200000 },
            @{ role='user'; content='我最喜欢的饮料是乌龙茶'; timestamp=1711015260000 }
        )
    }
    $add = Invoke-SmokePost '/v1/memory/add' $batch
    Assert-Smoke 'synchronous Add echoes contract' ($add.success -and $add.request_id -eq $batch.request_id -and $add.user_id -eq $user -and $add.session_id -eq $batch.session_id -and $add.stored -eq 5)
    $current = Find-Smoke 'What is my favorite color now?'
    Assert-Smoke 'current state and immediate recall' ($current.Contains('favorite color is red') -and -not $current.Contains('favorite color is blue'))
    Assert-Smoke 'contact minimization' (-not $current.Contains('alice@example.com') -and -not $current.Contains('415-555-1212'))
    $history = Find-Smoke 'What was my favorite color on 2024-02-10?'
    Assert-Smoke 'historical validity' ($history.Contains('favorite color is blue') -and $history.Contains('superseded'))
    $rules = Find-Smoke 'Apply the rule to label B'
    Assert-Smoke 'short examples preserved' ($rules.Contains('B=blue'))
    $choices = Find-Smoke 'Which statement matches the record?' @('A. orbital-code42 is the telemetry identifier', 'B. never-stored-code99 is the telemetry identifier')
    Assert-Smoke 'choice hints return stored evidence only' ($choices.Contains('orbital-code42') -and -not $choices.Contains('never-stored-code99'))
    $chinese = Find-Smoke '用户最喜欢的饮料是什么'
    Assert-Smoke 'UTF-8 Chinese evidence' ($chinese.Contains('乌龙茶'))
    $profile = Find-Smoke 'What do you know about me?'
    Assert-Smoke 'generic profile retains relevant user evidence' ($profile.Contains('favorite color is red') -and $profile.Contains('乌龙茶'))
    $calendar = Find-Smoke 'What happened on 2024-03-21?'
    Assert-Smoke 'date-only view retains event evidence' ($calendar.Contains('orbital-code42'))
    $isolation = Invoke-SmokePost '/v1/memory/search' @{ user_id="$user-other"; query='orbital-code42'; top_k=20 }
    Assert-Smoke 'user isolation' (@($isolation.data).Count -eq 0)
    Invoke-SmokePost '/v1/memory/add' $batch | Out-Null
    $afterReplay = Find-Smoke 'What is my favorite color now?'
    Assert-Smoke 'HTTP replay preserves newest state' ($afterReplay.Contains('favorite color is red') -and -not $afterReplay.Contains('favorite color is blue'))
} finally {
    $checks | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $PSScriptRoot $ResultsFile) -Encoding utf8
    $headers.Clear()
}
$checks | Format-Table -AutoSize
