param(
    [Parameter(Mandatory)][string]$BaseUrl,
    [Parameter(Mandatory)][string]$MemoryToken,
    [ValidatePattern('^[a-zA-Z0-9._-]+[.]json$')][string]$ResultsFile='fullfocus-live-smoke-results.json'
)
$ErrorActionPreference='Stop'
if (([Uri]$BaseUrl).Host -notin @('localhost','127.0.0.1')) { throw 'Only a disposable loopback server is allowed' }
$headers=@{Authorization="Bearer $MemoryToken"}
$user='local-fullfocus-'+[Guid]::NewGuid().ToString('N')
$checks=[System.Collections.Generic.List[object]]::new()
function Assert-Full([string]$Name,[bool]$Passed) {
    $checks.Add([pscustomobject]@{check=$Name;passed=$Passed})
    if (-not $Passed) {throw "Full-focus assertion failed: $Name"}
}
function Invoke-FullPost([string]$Path,$Body) {
    Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Post -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ($Body|ConvertTo-Json -Depth 12 -Compress) -TimeoutSec 30
}
function Add-Full([string]$Session,[array]$Messages,[string]$UserId=$user) {
    $request=[Guid]::NewGuid().ToString('N')
    $result=Invoke-FullPost '/v1/memory/add' @{user_id=$UserId;session_id=$Session;request_id=$request;messages=$Messages}
    Assert-Full 'Add synchronous acknowledgement and identifiers' ($result.success -and $result.stored -eq $Messages.Count -and $result.user_id -eq $UserId -and $result.session_id -eq $Session -and $result.request_id -eq $request)
}
function Find-Full([string]$Query,[int]$TopK=20,[string]$UserId=$user) {
    $result=Invoke-FullPost '/v1/memory/search' @{user_id=$UserId;query=$Query;top_k=$TopK}
    Assert-Full 'top_k respected' (@($result.data).Count -le $TopK)
    return (@($result.data)|ForEach-Object content)-join "`n"
}
try {
    $health=Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 5
    Assert-Full 'health' ($health.status -eq 'ok')
    Add-Full 'identity' @(
        @{role='user';message_id='phone-old';content='My phone number is 13812345678'},
        @{role='user';message_id='phone-new';content='My phone number is 13912345678'},
        @{role='user';message_id='color';content='My favorite color is jade'}
    )
    Add-Full 'privacy' @(@{role='user';message_id='forget-phone';content='Please forget my phone number'})
    foreach ($query in @('What is my phone number?','What was my previous phone number?','Summarize all conversations')) {
        $text=Find-Full $query
        Assert-Full 'forgotten evidence absent from current history and transcript' (-not $text.Contains('13812345678') -and -not $text.Contains('13912345678') -and -not $text.Contains('lifecycle control'))
    }
    Add-Full 'quoted' @(
        @{role='assistant';message_id='assistant';content='Forget my favorite color'},
        @{role='user';message_id='quote';content='"Forget my favorite color" is a quoted label.'}
    )
    $text=Find-Full 'What is my favorite color?'
    Assert-Full 'deletion remains scoped and quoted commands are data' ($text.Contains('color is jade'))
    Add-Full 'suppression' @(
        @{role='user';message_id='old';content='My email is old@example.com'},
        @{role='user';message_id='stop';content='Do not store my email anymore'},
        @{role='user';message_id='blocked';content='My email is blocked@example.com'}
    )
    $text=Find-Full 'What is my email?'
    Assert-Full 'persistent suppression blocks new matching facts' (-not $text.Contains('@example.com'))
    Add-Full 'suppression' @(
        @{role='user';message_id='resume';content='You may remember my email again'},
        @{role='user';message_id='fresh';content='My email is fresh@example.com'}
    )
    $text=Find-Full 'What is my email now?'
    Assert-Full 'explicit resume accepts fresh data without reviving deleted data' ($text.Contains('fresh@example.com') -and -not $text.Contains('old@example.com') -and -not $text.Contains('blocked@example.com'))
    $json='{"project":"Aster","owners":["Mira","Noah"],"limits":{"red":2,"blue":3}}'
    Add-Full 'structure' @(@{role='user';message_id='json';content=$json})
    $text=Find-Full 'List all owners and limits for Aster' 1
    $jsonStart=$text.IndexOf('{')
    $parsed=if ($jsonStart -ge 0) {$text.Substring($jsonStart)|ConvertFrom-Json} else {$null}
    Assert-Full 'JSON fields and collection preserved in one result' ($parsed.project -eq 'Aster' -and $parsed.owners.Count -eq 2 -and $parsed.limits.blue -eq 3)
    Add-Full 'workflow' @(@{role='user';message_id='steps';content="Aster deployment workflow`n1. Stop the worker.`n2. Copy the snapshot.`n3. Restart the worker."})
    $text=Find-Full 'Which Aster deployment workflow steps apply?' 1
    Assert-Full 'workflow headers and ordered steps preserved' ($text.Contains('1. Stop') -and $text.IndexOf('2. Copy') -gt $text.IndexOf('1. Stop') -and $text.IndexOf('3. Restart') -gt $text.IndexOf('2. Copy'))
    Add-Full 'path1' @(@{role='user';message_id='p1';content='Nova handed parcel47 to Vega.'})
    Add-Full 'path2' @(@{role='user';message_id='p2';content='Vega transferred the same package to Lyra.'})
    Add-Full 'path3' @(@{role='user';message_id='p3';content='Lyra stored it in vault nine.'})
    Add-Full 'other-path' @(@{role='user';message_id='other';content='Vega stored parcel47 in the forbidden vault.'}) "$user-other"
    $text=Find-Full "Where did Nova's parcel47 end up?" 3
    Assert-Full 'two-hop support crosses sessions with original sources' ($text.Contains('Nova handed') -and $text.Contains('Vega transferred') -and $text.Contains('vault nine') -and $text.Contains('linked_source=') -and -not $text.Contains('forbidden'))
    Add-Full 'background' @(
        @{role='user';message_id='self';content='I am allergic to peanuts.'},
        @{role='user';message_id='friend';content='My friend Mira is allergic to strawberries.'}
    )
    $text=Find-Full 'Suggest a snack for me'
    Assert-Full 'relevant personal background does not borrow friend health' ($text.Contains('peanuts') -and -not $text.Contains('strawberries'))
    Add-Full 'formats' @(
        @{role='user';message_id='a1';content='For project Aster, always answer in JSON.'},
        @{role='user';message_id='b1';content='For project Birch, always answer in XML.'},
        @{role='user';message_id='a2';content='For project Aster, always answer in YAML.'}
    )
    $text=Find-Full 'What response format applies to project Aster?'
    Assert-Full 'same-project format updates and cross-project exclusions' ($text.Contains('YAML') -and -not $text.Contains('answer in JSON') -and -not $text.Contains('XML'))
    $text=Find-Full 'What response format applies to project Birch?'
    Assert-Full 'other project format survives independent update' ($text.Contains('XML') -and -not $text.Contains('YAML'))
    Add-Full 'local-state' @(
        @{role='user';message_id='old';content='My favorite color is red';timestamp='2024-01-01T09:00:00+08:00'},
        @{role='user';message_id='new';content='My favorite color is blue';timestamp='2024-03-20T00:00:00+08:00'}
    ) "$user-clock"
    $text=Find-Full 'What was my favorite color on 2024-03-19?' 100 "$user-clock"
    Assert-Full 'historical query respects source local midnight' ($text.Contains('color is red') -and -not $text.Contains('color is blue'))
    $text=Find-Full 'What was my favorite color on 2024-03-20?' 100 "$user-clock"
    Assert-Full 'expired state excluded at exact midnight boundary' ($text.Contains('color is blue') -and -not $text.Contains('color is red'))
    $text=Find-Full 'Why did unseen-Zeta reach unknown-Orion?'
    Assert-Full 'no evidence seed remains empty' ([string]::IsNullOrEmpty($text))
    Add-Full 'clear' @(@{role='user';message_id='clear';content='Forget everything you know about me'})
    $text=Find-Full 'Summarize all conversations' 100
    Assert-Full 'forget all leaves no searchable transcript' ([string]::IsNullOrEmpty($text))
    $text=Find-Full 'Where is the parcel47 that Vega stored?' 20 "$user-other"
    Assert-Full 'forget all never crosses user scope' ($text.Contains('forbidden vault'))
} finally {
    [pscustomobject]@{created_at=(Get-Date).ToString('o');checks=$checks.ToArray();passed=@($checks|Where-Object passed).Count;failed=@($checks|Where-Object {-not $_.passed}).Count}|ConvertTo-Json -Depth 6|Set-Content (Join-Path $PSScriptRoot $ResultsFile) -Encoding utf8
}
Write-Output "$($checks.Count) full-focus HTTP assertions passed"
