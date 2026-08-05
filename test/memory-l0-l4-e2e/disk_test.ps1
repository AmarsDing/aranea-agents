Get-ChildItem F:\aranea-agents\test -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $size = (Get-ChildItem $_.FullName -Recurse -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
    [PSCustomObject]@{ Dir = $_.Name; SizeMB = [math]::Round($size/1MB,1); LastWrite = $_.LastWriteTime }
} | Sort-Object SizeMB -Descending | Select-Object -First 20 | Format-Table -AutoSize
