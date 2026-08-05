$ErrorActionPreference = 'Stop'
function Get-Traces($qs) {
  Invoke-RestMethod -Uri "http://127.0.0.1:8000/v1/monitor/traces?$qs" -Method Get
}
$r = Get-Traces 'limit=5'
"default total=$($r.total)"
"domainCounts: $(($r.domainCounts.PSObject.Properties | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join ' ')"
"statusCounts: $(($r.statusCounts.PSObject.Properties | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join ' ')"
"firstNames: $(($r.items | ForEach-Object { $_.name + '/' + $_.status }) -join ', ')"
$r2 = Get-Traces 'limit=5&exclude_internal=true'
"exclude_internal total=$($r2.total) names=$(($r2.items | ForEach-Object { $_.name }) -join ',')"
$r3 = Get-Traces 'limit=5&domain=system'
"domain=system total=$($r3.total)"
$r4 = Get-Traces 'limit=5&status=ok&exclude_internal=true'
"status=ok+excl total=$($r4.total)"
$r5 = Get-Traces 'limit=5&exclude_internal=true&status=running'
"status=running+excl total=$($r5.total)"
