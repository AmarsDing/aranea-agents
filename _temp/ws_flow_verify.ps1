# WS E2E verification (single process): connect global monitor stream,
# enable process log, submit chat message, capture flow_log / log events.
$ErrorActionPreference = 'Stop'

$wsUrl = 'ws://localhost:8000/v1/ws?session_id=*'
$outFile = 'F:\aranea-agents\_temp\ws_events.jsonl'
if (Test-Path $outFile) { Remove-Item $outFile -Force }

$ws = [System.Net.WebSockets.ClientWebSocket]::new()
$ws.ConnectAsync([Uri]$wsUrl, [Threading.CancellationToken]::None).Wait()
Write-Host "WS_STATE: $($ws.State)"

function Send-Up($obj) {
    $json = ($obj | ConvertTo-Json -Depth 6 -Compress)
    $bytes = [Text.Encoding]::UTF8.GetBytes($json)
    $seg = [ArraySegment[byte]]::new($bytes)
    $ws.SendAsync($seg, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [Threading.CancellationToken]::None).Wait()
    Write-Host "SENT: $json"
}

Send-Up @{ direction = 'client_to_server'; channel = 'system'; type = 'subscribe'; payload = @{ channel = 'monitor' } }
Send-Up @{ direction = 'client_to_server'; channel = 'system'; type = 'enable_log'; payload = @{ enabled = $true } }

# submit chat message (fire-and-ack async command)
$body = @{
    session_id = '7a13e209-df10-47c0-888d-38c6ed70261d'
    content    = 'flow-log-verify ping'
    options    = @{ dialog_mode = 'default' }
} | ConvertTo-Json -Depth 5
try {
    $resp = Invoke-RestMethod -Uri 'http://localhost:8000/v1/chat/messages/submit' -Method Post -Body $body -ContentType 'application/json' -TimeoutSec 15
    Write-Host "SUBMIT_RESP: $($resp | ConvertTo-Json -Compress)"
} catch {
    Write-Host "SUBMIT_ERR: $($_.Exception.Message)"
}

# receive loop in same process: 40s total, 5s per-receive timeout
$total = 0; $flow = 0; $proc = 0; $other = 0
$flowSamples = @(); $procSamples = @(); $otherSamples = @(); $otherTypes = @{}
$sw = [Diagnostics.Stopwatch]::StartNew()
$buf = New-Object byte[] 65536
while ($sw.Elapsed.TotalSeconds -lt 40 -and $ws.State -eq 'Open') {
    $cts = [Threading.CancellationTokenSource]::new(5000)
    $seg = [ArraySegment[byte]]::new($buf)
    $ms = New-Object IO.MemoryStream
    $got = $false
    try {
        do {
            $res = $ws.ReceiveAsync($seg, $cts.Token).GetAwaiter().GetResult()
            if ($res.MessageType -eq 'Close') { break }
            $ms.Write($buf, 0, $res.Count)
            $got = $true
        } while (-not $res.EndOfMessage)
    } catch [OperationCanceledException] {
        # per-receive timeout; check stopwatch
    }
    if ($got -and $ms.Length -gt 0) {
        $line = [Text.Encoding]::UTF8.GetString($ms.ToArray())
        Add-Content -Path $outFile -Value $line -Encoding UTF8
        $total++
        try { $ev = $line | ConvertFrom-Json } catch { continue }
        $me = $ev.monitor_event
        $t = if ($null -ne $me) { $me.type } else { $ev.type }
        if ($t -eq 'flow_log') {
            $flow++
            $step = $me.metadata.flow_step; if (-not $step) { $step = $me.metadata.step_id }
            if ($flowSamples.Count -lt 10) { $flowSamples += "$step | $($me.metadata.phase) | $($me.message)" }
        } elseif ($t -eq 'log') {
            $proc++
            if ($procSamples.Count -lt 3) { $procSamples += "$($me.level) | $($me.message)" }
        } else {
            $other++
            $key = if ($null -eq $t) { '(null)' } else { [string]$t }
            if ($otherSamples.Count -lt 3) { $otherSamples += $line.Substring(0, [Math]::Min(300, $line.Length)) }
            if ($otherTypes.ContainsKey($key)) { $otherTypes[$key] += 1 } else { $otherTypes[$key] = 1 }
        }
    }
    $ms.Dispose()
    $cts.Dispose()
}
try { $ws.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, 'done', [Threading.CancellationToken]::None).Wait() } catch {}
$ws.Dispose()

Write-Host "==== RESULT ===="
Write-Host "TOTAL_FRAMES: $total"
Write-Host "FLOW_LOG_EVENTS: $flow"
Write-Host "PROCESS_LOG_EVENTS: $proc"
Write-Host "OTHER_FRAMES: $other"
Write-Host "-- flow samples --"
$flowSamples | ForEach-Object { Write-Host $_ }
Write-Host "-- process samples --"
$procSamples | ForEach-Object { Write-Host $_ }
Write-Host "-- other types --"
$otherTypes.GetEnumerator() | ForEach-Object { Write-Host "$($_.Key): $($_.Value)" }
Write-Host "-- other raw samples --"
$otherSamples | ForEach-Object { Write-Host $_ }
