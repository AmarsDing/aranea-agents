Get-ChildItem F:\ -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $size = (Get-ChildItem $_.FullName -Recurse -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
    [PSCustomObject]@{ Dir = $_.FullName; SizeGB = [math]::Round($size/1GB,2) }
} | Sort-Object SizeGB -Descending | Select-Object -First 15 | Format-Table -AutoSize
