$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

function Test-ToolDetail($id, $innerArgsJson, $desc) {
    Write-Host "=== $desc ===" -ForegroundColor Cyan
    $body = ('{"arguments_json":' + ($innerArgsJson | ConvertTo-Json -Compress) + ',"timeout_sec":30}')
    Write-Host "Body: $body"

    try {
        $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/$id/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
        Write-Host "Status: $($resp.status)"
        Write-Host "Error: $($resp.error_message)"
        Write-Host "Duration: $($resp.duration_ms)"
        Write-Host "Preview: $($resp.result_preview)"
    } catch {
        $err = $_.Exception
        if($err.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
                $errBody = $reader.ReadToEnd()
                Write-Host "HTTP Error: $errBody"
            } catch {
                Write-Host "Error: $($err.Message)"
            }
        } else {
            Write-Host "Error: $($err.Message)"
        }
    }
    Write-Host ""
}

# Test failing file tools with detail
Test-ToolDetail "tool_save_file" '{"path":"_test_tool_output.txt","content":"hello from tool test"}' "save_file"
Test-ToolDetail "tool_search_content" '{"query":"func main","path":"."}' "search_content"
Test-ToolDetail "tool_read_multiple_files" '{"paths":["go.mod"]}' "read_multiple_files"
Test-ToolDetail "tool_replace_content" '{"file_name":"_test_tool_output.txt","old_string":"hello","new_string":"world"}' "replace_content"
Test-ToolDetail "tool_diff_edit" '{"file_name":"_test_tool_output.txt","edits":[{"search":"world","replace":"hello"}]}' "diff_edit"

# Test CLI admin with detail
Test-ToolDetail "tool_cli_admin_agent_list" '{"limit":5}' "cli_admin_agent_list"
Test-ToolDetail "tool_kanban" '{"action":"show"}' "kanban"
Test-ToolDetail "tool_browser" '{}' "browser"
