$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Test cli_admin_agent_list with detailed error capture
$body = '{"arguments_json":"{\"limit\":5}","timeout_sec":30}'
Write-Host "Testing cli_admin_agent_list..."
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_cli_admin_agent_list/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
    Write-Host "Response: $($resp | ConvertTo-Json -Depth 3)"
} catch {
    $err = $_.Exception
    Write-Host "Exception type: $($err.GetType().Name)"
    Write-Host "Message: $($err.Message)"
    if($err.Response) {
        Write-Host "Status: $($err.Response.StatusCode)"
        Write-Host "StatusDesc: $($err.Response.StatusDescription)"
        try {
            $stream = $err.Response.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($stream)
            $errBody = $reader.ReadToEnd()
            Write-Host "Body: $errBody"
        } catch {
            Write-Host "Could not read body: $_"
        }
    }
}
