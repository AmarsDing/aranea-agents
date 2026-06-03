$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Use raw single-line JSON strings (this format works with Invoke-RestMethod)
$tests = @(
    @{id="tool_read_file"; body='{"arguments_json":"{\"file_name\":\"README.md\"}","timeout_sec":30}'; desc="read_file"},
    @{id="tool_save_file"; body='{"arguments_json":"{\"path\":\"_test_tool_output.txt\",\"content\":\"hello from tool test\"}","timeout_sec":30}'; desc="save_file"},
    @{id="tool_search_content"; body='{"arguments_json":"{\"query\":\"func main\",\"path\":\".\"}","timeout_sec":30}'; desc="search_content"},
    @{id="tool_search_file"; body='{"arguments_json":"{\"pattern\":\"*.go\",\"path\":\".\"}","timeout_sec":30}'; desc="search_file"},
    @{id="tool_read_multiple_files"; body='{"arguments_json":"{\"paths\":[\"go.mod\"]}","timeout_sec":30}'; desc="read_multiple_files"},
    @{id="tool_list_file"; body='{"arguments_json":"{\"path\":\".\",\"pattern\":\"*.md\"}","timeout_sec":30}'; desc="list_file"},
    @{id="tool_replace_content"; body='{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"old_string\":\"hello\",\"new_string\":\"world\"}","timeout_sec":30}'; desc="replace_content"},
    @{id="tool_diff_edit"; body='{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"edits\":[{\"search\":\"world\",\"replace\":\"hello\"}]}","timeout_sec":30}'; desc="diff_edit"},
    @{id="tool_patch_file"; body='{"arguments_json":"{\"file_name\":\"_test_tool_output.txt\",\"patch\":\"test\"}","timeout_sec":30}'; desc="patch_file"},
    @{id="tool_cli_admin_agent_list"; body='{"arguments_json":"{\"limit\":5}","timeout_sec":30}'; desc="cli_admin_agent_list"},
    @{id="tool_cli_admin_skill_list"; body='{"arguments_json":"{}","timeout_sec":30}'; desc="cli_admin_skill_list"},
    @{id="tool_kanban"; body='{"arguments_json":"{\"action\":\"show\"}","timeout_sec":30}'; desc="kanban"},
    @{id="tool_browser"; body='{"arguments_json":"{}","timeout_sec":30}'; desc="browser"},
    @{id="tool_mcp_broker"; body='{"arguments_json":"{\"action\":\"list_servers\"}","timeout_sec":30}'; desc="mcp_broker"}
)

$pass = 0
$fail = 0
$results = @()

foreach($t in $tests) {
    Write-Host "=== $($t.desc) ===" -ForegroundColor Cyan
    try {
        $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/$($t.id)/test" -Method Post -Headers $headers -Body $t.body -TimeoutSec 35
        $status = $resp.status
        $errMsg = $resp.error_message
        $preview = if($resp.result_preview) {$resp.result_preview.Substring(0, [Math]::Min(300, $resp.result_preview.Length))} else {""}
        $duration = $resp.duration_ms

        Write-Host "  Status: $status | Duration: $duration ms"
        if($errMsg) { Write-Host "  Error: $errMsg" -ForegroundColor Red }
        if($preview) { Write-Host "  Preview: $preview" }

        if($status -eq "success") { $pass++ } else { $fail++ }
        $results += @{desc=$t.desc; status=$status; error=$errMsg}
    } catch {
        $err = $_.Exception
        $errDetail = ""
        if($err.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($err.Response.GetResponseStream())
                $errDetail = $reader.ReadToEnd()
            } catch {}
        }
        Write-Host "  HTTP ERROR: $errDetail" -ForegroundColor Red
        $fail++
        $results += @{desc=$t.desc; status="http_error"; error=$errDetail}
    }
    Write-Host ""
}

Write-Host "====================================" -ForegroundColor Yellow
Write-Host "Results: $pass passed, $fail failed" -ForegroundColor Yellow
Write-Host "====================================" -ForegroundColor Yellow
foreach($r in $results) {
    $color = if($r.status -eq "success"){"Green"}else{"Red"}
    $errStr = if($r.error -and $r.error.Length -gt 0){" | $($r.error.Substring(0, [Math]::Min(80, $r.error.Length)))"}else{""}
    Write-Host "$($r.status.ToUpper()) | $($r.desc)$errStr" -ForegroundColor $color
}
