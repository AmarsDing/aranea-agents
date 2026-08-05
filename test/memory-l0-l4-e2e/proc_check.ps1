Get-CimInstance Win32_Process -Filter "Name like '%admin%'" | Select-Object ProcessId, Name, CreationDate, ExecutablePath | Format-List
