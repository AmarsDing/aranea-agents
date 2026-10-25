param(
    [Parameter(Mandatory)][string]$BaseUrl,
    [Parameter(Mandatory)][string]$MemoryToken,
    [ValidatePattern('^[a-zA-Z0-9._-]+[.]json$')][string]$ResultsFile='quality-live-smoke-results.json'
)
$ErrorActionPreference='Stop'
if (([Uri]$BaseUrl).Host -notin @('localhost','127.0.0.1')) { throw 'Only a disposable loopback server is allowed' }
$headers=@{Authorization="Bearer $MemoryToken"}
$user='local-quality-'+[Guid]::NewGuid().ToString('N')
$checks=[System.Collections.Generic.List[object]]::new()
function Assert-Quality([string]$Name,[bool]$Passed) {
    $checks.Add([pscustomobject]@{check=$Name;passed=$Passed})
    if (-not $Passed) {throw "Quality assertion failed: $Name"}
}
function Invoke-QualityPost([string]$Path,$Body) {
    Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Post -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ($Body|ConvertTo-Json -Depth 10 -Compress) -TimeoutSec 30
}
function Find-Quality([string]$Query,[int]$TopK=20) {
    $result=Invoke-QualityPost '/v1/memory/search' @{user_id=$user;query=$Query;top_k=$TopK}
    Assert-Quality 'top_k respected' (@($result.data).Count -le $TopK)
    return (@($result.data)|ForEach-Object content)-join "`n"
}
try {
    $health=Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 5
    Assert-Quality 'health' ($health.status -eq 'ok')
    $denied=$false
    try {Invoke-RestMethod -Uri "$BaseUrl/v1/memory/search" -Method Post -ContentType 'application/json' -Body '{"user_id":"unauthorized","query":"marker"}' -TimeoutSec 5|Out-Null}
    catch {$denied=$_.Exception.Response.StatusCode -eq 401}
    Assert-Quality 'authentication required' $denied
    $batch=@{
        request_id="$user-request";user_id=$user;session_id="$user-session"
        messages=@(
            @{role='user';message_id='old';content='My favorite color is red';timestamp='2024-01-01T09:00:00Z'},
            @{role='user';message_id='latest';content='My favorite color is blue';timestamp='2024-03-01T09:00:00Z'},
            @{role='user';message_id='middle';content='My favorite color is green';timestamp='2024-02-01T09:00:00Z'},
            @{role='user';message_id='home';content='I live in Rome';timestamp='2024-03-20T10:00:00Z'},
            @{role='user';message_id='past-home';content='I used to live in Berlin';timestamp='2024-03-21T10:00:00Z'},
            @{role='system';message_id='rule';content="Rule: calibration labels follow these examples.`nA=red`nB=blue`nC=green";timestamp='2024-03-21T11:00:00Z'},
            @{role='user';message_id='sample1';content='h(1)=6';timestamp='2024-03-22T10:00:00Z'},
            @{role='user';message_id='sample2';content='h (2)=11';timestamp='2024-03-22T11:00:00Z'},
            @{role='user';message_id='sample3';content='h(3)=16';timestamp='2024-03-22T12:00:00Z'},
            @{role='user';message_id='privacy';content='I live at 123 Maple Street, Rome. My password is velvet-test-47.';timestamp='2024-03-23T10:00:00Z'},
            @{role='user';message_id='diary';content="2024-02-06：`nWe calibrated the spectrometer.`nThe prism passed inspection.";timestamp='2024-03-24T10:00:00Z'},
            @{role='user';message_id='local-clock';content='Yesterday I inspected the quartz prism.';timestamp='2024-03-26T00:30:00+08:00'}
        )
    }
    $add=Invoke-QualityPost '/v1/memory/add' $batch
    Assert-Quality 'synchronous Add echoes contract' ($add.success -and $add.stored -eq 12 -and $add.request_id -eq $batch.request_id -and $add.user_id -eq $user -and $add.session_id -eq $batch.session_id)
    $current=Find-Quality 'What is my favorite color now?'
    Assert-Quality 'late middle state preserves current' ($current.Contains('favorite color is blue') -and -not $current.Contains('favorite color is green') -and -not $current.Contains('favorite color is red'))
    $history=Find-Quality 'What was my favorite color on 2024-02-15?'
    Assert-Quality 'as-of interval chain has unique middle state' ($history.Contains('favorite color is green') -and -not $history.Contains('favorite color is red') -and -not $history.Contains('favorite color is blue'))
    $city=Find-Quality 'Which city do I live in now?'
    Assert-Quality 'past residence does not replace current' ($city.Contains('Rome') -and -not $city.Contains('Berlin'))
    Assert-Quality 'city query minimizes street and credential disclosure' (-not $city.Contains('123 Maple Street') -and -not $city.Contains('velvet-test-47'))
    $past=Find-Quality 'Where did I previously live?'
    Assert-Quality 'retrospective residence remains retrievable' ($past.Contains('Berlin'))
    $rule=Find-Quality 'Apply the calibration rule to label B' 1
    Assert-Quality 'one evidence slot retains rule and demonstrations' ($rule.Contains('Rule:') -and $rule.Contains('B=blue') -and $rule.Contains('C=green'))
    $transfer=Find-Quality 'What is h(8)?'
    Assert-Quality 'learned function returns original demonstrations' ($transfer.Contains('h(1)=6') -and $transfer.Contains('h (2)=11') -and $transfer.Contains('h(3)=16') -and -not $transfer.Contains('h(8)=41'))
    $diary=Find-Quality 'Which prism inspection happened on 2024-02-06?'
    Assert-Quality 'date heading dates later diary atoms' ($diary.Contains('prism passed inspection') -and $diary.Contains('2024-02-06'))
    $local=Find-Quality 'What happened yesterday?'
    Assert-Quality 'source timezone anchors calendar day' ($local.Contains('quartz prism') -and $local.Contains('2024-03-25'))
    $credential=Find-Quality 'What is my password?'
    Assert-Quality 'credential assignment always masked' ($credential.Contains('redacted') -and -not $credential.Contains('velvet-test-47'))
    $other=Invoke-QualityPost '/v1/memory/search' @{user_id="$user-other";query='What is h(8)?';top_k=20}
    Assert-Quality 'function retrieval preserves user isolation' (@($other.data).Count -eq 0)
    Invoke-QualityPost '/v1/memory/add' $batch|Out-Null
    $replay=Find-Quality 'What is my favorite color now?'
    Assert-Quality 'replay preserves current state' ($replay.Contains('favorite color is blue') -and -not $replay.Contains('favorite color is green'))
} finally {
    $checks|ConvertTo-Json -Depth 5|Set-Content (Join-Path $PSScriptRoot $ResultsFile) -Encoding utf8
    $headers.Clear()
}
$checks|Format-Table -AutoSize
