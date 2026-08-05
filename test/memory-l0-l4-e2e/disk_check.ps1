Get-PSDrive F | Select-Object @{N='FreeMB';E={[math]::Round($_.Free/1MB,1)}} | Format-Table -AutoSize
