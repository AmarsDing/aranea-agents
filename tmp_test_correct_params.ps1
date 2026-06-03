$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

function Test-Tool($id, $body, $desc) {
    Write-Host "=== $desc ===" -ForegroundColor Cyan
    try {
        $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/$id/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
        $status = $resp.status
        $errMsg = $resp.error_message
        $preview = if($resp.result_preview) {$resp.result_preview.Substring(0, [Math]::Min(300, $resp.result_preview.Length))} else {""}
        $duration = $resp.duration_ms

        if($status -eq "success") {
            Write-Host "  PASS ($duration ms)" -ForegroundColor Green
            if($preview) { Write-Host "  Preview: $preview" }
        } else {
            Write-Host "  FAIL ($duration ms): $errMsg" -ForegroundColor Red
            if($preview) { Write-Host "  Preview: $preview" }
        }
    } catch {
        $err = $_.Exception
        $errDetail = ""
        if($err.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
                $errDetail = $reader.ReadToEnd()
            } catch {}
        }
        Write-Host "  ERROR: $($err.Message)" -ForegroundColor Red
        if($errDetail) { Write-Host "  Detail: $errDetail" }
    }
    Write-Host ""
}

# Test with CORRECT parameter names (matching Go struct JSON tags)
Test-Tool "tool_save_file" '{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"contents\":\"hello from tool test\"}","timeout_sec":30}' "save_file (correct params: file_name+contents)"
Test-Tool "tool_read_multiple_files" '{"arguments_json":"{\"patterns\":[\"go.mod\"]}","timeout_sec":30}' "read_multiple_files (correct params: patterns)"
Test-Tool "tool_search_content" '{"arguments_json":"{\"content_pattern\":\"func main\",\"path\":\".\"}","timeout_sec":30}' "search_content (correct params: content_pattern)"

# Test file modification tools with pre-existing file
Test-Tool "tool_replace_content" '{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"old_string\":\"hello\",\"new_string\":\"world\"}","timeout_sec":30}' "replace_content"
Test-Tool "tool_diff_edit" '{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"edits\":[{\"search\":\"world\",\"replace\":\"hello\"}]}","timeout_sec":30}' "diff_edit"

# Also test the already-working tools
Test-Tool "tool_read_file" '{"arguments_json":"{\"file_name\":\"README.md\"}","timeout_sec":30}' "read_file"
Test-Tool "tool_search_file" '{"arguments_json":"{\"pattern\":\"*.go\",\"path\":\".\"}","timeout_sec":30}' "search_file"
Test-Tool "tool_list_file" '{"arguments_json":"{\"path\":\".\",\"pattern\":\"*.md\"}","timeout_sec":30}' "list_file"
