Get-CimInstance Win32_Process -Filter "Name like '%launcher%' or Name like '%aranea%'" | Select-Object ProcessId, Name, CreationDate, ExecutablePath, CommandLine | Format-List
Get-Item D:\aranea-runtime\admin.exe | Select-Object FullName, LastWriteTime
