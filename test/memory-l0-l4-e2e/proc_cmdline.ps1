Get-CimInstance Win32_Process -Filter "Name = 'admin.exe'" | Select-Object ProcessId, CommandLine | Format-List
