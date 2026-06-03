$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Test with raw JSON body
$body = '{"arguments_json":"{\"file_name\":\"README.md\"}","timeout_sec":30}'
Write-Host "Testing read_file with raw JSON..."
Write-Host "Body: $body"
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_read_file/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
    Write-Host "Status: $($resp.status)"
    Write-Host "Error: $($resp.error_message)"
    Write-Host "Preview: $($resp.result_preview)"
    Write-Host "Duration: $($resp.duration_ms)"
} catch {
    $err = $_.Exception
    if($err.Response) {
        try {
            $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
            $errBody = $reader.ReadToEnd()
            Write-Host "Error response: $errBody"
        } catch {
            Write-Host "Error: $($err.Message)"
        }
    } else {
        Write-Host "Error: $($err.Message)"
    }
}

Write-Host ""
Write-Host "--- Testing with ConvertTo-Json ---"
$innerArgs = @{file_name="README.md"} | ConvertTo-Json -Depth 3 -Compress
$bodyObj = @{arguments_json=$innerArgs; timeout_sec=30} | ConvertTo-Json -Depth 3
Write-Host "Body: $bodyObj"
try {
    $resp2 = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_read_file/test" -Method Post -Headers $headers -Body $bodyObj -TimeoutSec 35
    Write-Host "Status: $($resp2.status)"
    Write-Host "Error: $($resp2.error_message)"
    Write-Host "Preview: $($resp2.result_preview)"
} catch {
    $err = $_.Exception
    if($err.Response) {
        try {
            $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
            $errBody = $reader.ReadToEnd()
            Write-Host "Error response: $errBody"
        } catch {
            Write-Host "Error: $($err.Message)"
        }
    } else {
        Write-Host "Error: $($err.Message)"
    }
}
