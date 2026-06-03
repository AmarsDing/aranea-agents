$body = Get-Content "f:\aranea-agents\tmp_agent.json" -Raw
Invoke-RestMethod -Uri "http://localhost:8000/v1/agents" -Method Post -Headers @{"X-User-ID"="dev"; "Content-Type"="application/json"} -Body $body | ConvertTo-Json -Depth 3
