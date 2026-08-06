# Parallel ranged downloader for slow GitHub mirror links.
param(
    [string]$Url,
    [string]$OutFile,
    [long]$Size,
    [int]$Parts = 8
)
$ErrorActionPreference = "Stop"
$chunk = [math]::Ceiling($Size / $Parts)
$jobs = @()
for ($i = 0; $i -lt $Parts; $i++) {
    $start = $i * $chunk
    $end = [math]::Min($start + $chunk - 1, $Size - 1)
    if ($start -gt $end) { break }
    $part = "$OutFile.part$i"
    $jobs += Start-Job -ScriptBlock {
        param($u, $p, $s, $e)
        curl.exe -sL --retry 3 -r "$s-$e" -o $p $u
        return (Get-Item $p).Length
    } -ArgumentList $Url, $part, $start, $end
}
$results = $jobs | Wait-Job | Receive-Job
$jobs | Remove-Job
$fs = [System.IO.File]::Create($OutFile)
for ($i = 0; $i -lt $Parts; $i++) {
    $part = "$OutFile.part$i"
    if (-not (Test-Path $part)) { continue }
    $bytes = [System.IO.File]::ReadAllBytes($part)
    $fs.Write($bytes, 0, $bytes.Length)
    Remove-Item $part
}
$fs.Close()
Write-Output ("merged size: " + (Get-Item $OutFile).Length)
