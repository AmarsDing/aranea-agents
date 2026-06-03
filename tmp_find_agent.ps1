$r = Invoke-RestMethod -Uri 'http://localhost:8000/v1/agents?limit=90' -Headers @{'X-User-ID'='dev'}
foreach($a in $r.agents){
    if($a.agentKey -eq 'tool-tester'){
        Write-Host "FOUND: $($a.id) $($a.agentKey)"
    }
}
if(-not ($r.agents | Where-Object { $_.agentKey -eq 'tool-tester' })){
    Write-Host "NOT FOUND - creating agent..."
    $body = Get-Content "f:\aranea-agents\tmp_agent.json" -Raw
    $result = Invoke-RestMethod -Uri "http://localhost:8000/v1/agents" -Method Post -Headers @{"X-User-ID"="dev"; "Content-Type"="application/json"} -Body $body
    Write-Host "Created: $($result.id) $($result.agentKey)"
}
