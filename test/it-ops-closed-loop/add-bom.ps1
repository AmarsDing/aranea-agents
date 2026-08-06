param([string]$Path)
$c = [System.IO.File]::ReadAllText($Path, (New-Object System.Text.UTF8Encoding($false)))
[System.IO.File]::WriteAllText($Path, $c, (New-Object System.Text.UTF8Encoding($true)))
Write-Output "BOM written: $Path"
