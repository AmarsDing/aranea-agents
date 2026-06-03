$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

$body = '{"arguments_json":"{\"content_pattern\":\"func main\",\"path\":\".\"}","timeout_sec":30}'
Write-Host "Testing search_content with content_pattern..."
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_search_content/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
    Write-Host "Status: $($resp.status)"
    Write-Host "Error: $($resp.error_message)"
    Write-Host "Preview: $($resp.result_preview)"
    Write-Host "Duration: $($resp.duration_ms)"
} catch {
    Write-Host "Error: $($_.Exception.Message)"
}

Write-Host ""
Write-Host "Testing search_content with file_pattern..."
$body2 = '{"arguments_json":"{\"content_pattern\":\"func main\",\"path\":\".\",\"file_pattern\":\"*.go\"}","timeout_sec":30}'
try {
    $resp2 = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_search_content/test" -Method Post -Headers $headers -Body $body2 -TimeoutSec 35
    Write-Host "Status: $($resp2.status)"
    Write-Host "Error: $($resp2.error_message)"
    Write-Host "Preview: $($resp2.result_preview)"
} catch {
    Write-Host "Error: $($_.Exception.Message)"
}
