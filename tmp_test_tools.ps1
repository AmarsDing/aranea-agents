$ErrorActionPreference = "Continue"
$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Define test cases for each tool
$testCases = @(
    @{id="tool_read_file"; args='{"path":"README.md"}'; desc="read_file - read README.md"},
    @{id="tool_save_file"; args='{"path":"_test_tool_output.txt","content":"hello from tool test"}'; desc="save_file - write test file"},
    @{id="tool_search_content"; args='{"path":".","pattern":"func main"}'; desc="search_content - search for func main"},
    @{id="tool_search_file"; args='{"path":".","pattern":"*.go"}'; desc="search_file - find Go files"},
    @{id="tool_read_multiple_files"; args='{"paths":["go.mod"]}'; desc="read_multiple_files - read go.mod"},
    @{id="tool_replace_content"; args='{"path":"_test_tool_output.txt","old":"hello","new":"world"}'; desc="replace_content - replace text"},
    @{id="tool_diff_edit"; args='{"path":"_test_tool_output.txt","edits":[{"oldText":"world","newText":"hello"}]}'; desc="diff_edit - edit file"},
    @{id="tool_patch_file"; args='{"path":"_test_tool_output.txt","patches":[{"op":"replace","path":"/content","value":"patched"}]}'; desc="patch_file - patch file"},
    @{id="tool_cli_admin_agent_list"; args='{}'; desc="cli_admin_agent_list - list agents"},
    @{id="tool_cli_admin_skill_list"; args='{}'; desc="cli_admin_skill_list - list skills"},
    @{id="tool_cli_admin_agent_get"; args='{"agent_id":"nonexistent"}'; desc="cli_admin_agent_get - get agent"},
    @{id="tool_cli_admin_skill_get"; args='{"skill_id":"nonexistent"}'; desc="cli_admin_skill_get - get skill"},
    @{id="tool_kanban"; args='{"action":"show"}'; desc="kanban - show board"},
    @{id="tool_browser"; args='{}'; desc="browser - basic test"},
    @{id="tool_mcp_broker"; args='{"action":"list_servers"}'; desc="mcp_broker - list servers"}
)

$results = @()
$pass = 0
$fail = 0

foreach($tc in $testCases) {
    Write-Host "Testing: $($tc.desc) ..." -ForegroundColor Cyan
    $body = @{
        arguments_json = $tc.args
        timeout_sec = 30
    } | ConvertTo-Json -Depth 3

    try {
        $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/$($tc.id)/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
        $status = $resp.status
        $errMsg = $resp.error_message
        $preview = $resp.result_preview
        $duration = $resp.duration_ms

        if($status -eq "success") {
            Write-Host "  PASS ($duration ms)" -ForegroundColor Green
            if($preview) { Write-Host "  Preview: $($preview.Substring(0, [Math]::Min(200, $preview.Length)))" }
            $pass++
        } else {
            Write-Host "  FAIL: $errMsg" -ForegroundColor Red
            $fail++
        }
        $results += @{desc=$tc.desc; status=$status; error=$errMsg; duration=$duration}
    } catch {
        $errMsg = $_.Exception.Message
        Write-Host "  ERROR: $errMsg" -ForegroundColor Red
        $fail++
        $results += @{desc=$tc.desc; status="error"; error=$errMsg; duration=0}
    }
    Write-Host ""
}

Write-Host "====================================" -ForegroundColor Yellow
Write-Host "Results: $pass passed, $fail failed" -ForegroundColor Yellow
Write-Host "====================================" -ForegroundColor Yellow

foreach($r in $results) {
    $color = if($r.status -eq "success"){"Green"}else{"Red"}
    Write-Host "$($r.status.ToUpper()) | $($r.desc) | $($r.error)" -ForegroundColor $color
}
