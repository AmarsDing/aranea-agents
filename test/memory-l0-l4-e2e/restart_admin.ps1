Stop-Process -Id 16864 -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
$count = (Get-CimInstance Win32_Process -Filter "Name = 'admin.exe'" | Measure-Object).Count
Write-Output "remaining admin.exe processes: $count"
