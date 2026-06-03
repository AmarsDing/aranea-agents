$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Test read_file with verbose error output
$body = '{"arguments_json":"{\"path\":\"README.md\"}","timeout_sec":30}'
Write-Host "Testing read_file..."
Write-Host "Request body: $body"
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_read_file/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
    Write-Host "Response: $($resp | ConvertTo-Json -Depth 3)"
} catch {
    $err = $_.Exception
    if($err.Response) {
        $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
        $errBody = $reader.ReadToEnd()
        Write-Host "Error response: $errBody"
    } else {
        Write-Host "Error: $($err.Message)"
    }
}

Write-Host ""
Write-Host "--- Testing cli_admin_agent_list ---"
$body2 = '{"arguments_json":"{}","timeout_sec":30}'
Write-Host "Request body: $body2"
try {
    $resp2 = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_cli_admin_agent_list/test" -Method Post -Headers $headers -Body $body2 -TimeoutSec 35
    Write-Host "Response: $($resp2 | ConvertTo-Json -Depth 3)"
} catch {
    $err = $_.Exception
    if($err.Response) {
        $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
        $errBody = $reader.ReadToEnd()
        Write-Host "Error response: $errBody"
    } else {
        Write-Host "Error: $($err.Message)"
    }
}
