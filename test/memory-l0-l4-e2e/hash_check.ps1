$a = (Get-FileHash F:\aranea-agents\bin\admin.exe -Algorithm SHA256).Hash
$b = (Get-FileHash D:\aranea-runtime\admin.exe -Algorithm SHA256).Hash
Write-Output "src : $a"
Write-Output "dest: $b"
Write-Output "identical: $($a -eq $b)"
