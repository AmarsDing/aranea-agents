$ErrorActionPreference = 'Continue'
$out = 'f:\myproject\aranea-agents\agent-browser-out.txt'
$js = Get-Content 'f:\myproject\aranea-agents\hover-probe.js' -Raw -Encoding utf8
$b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($js))
agent-browser --session ctxbudget-hover eval -b $b64 *> $out
Write-Output "done"
