$before = (Get-PSDrive F).Free
Start-Sleep -Seconds 5
$after = (Get-PSDrive F).Free
Write-Output ("free before: {0:N0} bytes, after 5s: {1:N0} bytes, delta: {2:N0}" -f $before, $after, ($after-$before))
Get-ChildItem F:\aranea-agents\logs -File -ErrorAction SilentlyContinue | Sort-Object Length -Descending | Select-Object -First 6 Name, @{N='MB';E={[math]::Round($_.Length/1MB,1)}}, LastWriteTime | Format-Table -AutoSize
