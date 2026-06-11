$file = 'f:\aranea-agents\internal\biz\agent_usecase.go'
$content = [System.IO.File]::ReadAllText($file)
$content = $content -replace 'u\.reader\.', 'u.repo.'
$content = $content -replace 'u\.writer\.', 'u.repo.'
$content = $content -replace 'u\.settings\.', 'u.repo.'
$content = $content -replace 'u\.files\.', 'u.repo.'
$content = $content -replace 'u\.position\.', 'u.repo.'
$content = $content -replace 'u\.tx\.', 'u.repo.'
[System.IO.File]::WriteAllText($file, $content)
Write-Host "Done"
