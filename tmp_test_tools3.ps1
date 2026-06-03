$ErrorActionPreference = "Continue"
$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

function Test-Tool($id, $argsJson, $desc) {
    Write-Host "Testing: $desc ..." -ForegroundColor Cyan
    $body = "{`"arguments_json`":`"$($argsJson -replace '"','\"')`",`"timeout_sec`":30}"

    try {
        $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/$id/test" -Method Post -Headers $headers -Body $body -TimeoutSec 35
        $status = $resp.status
        $errMsg = $resp.error_message
        $preview = $resp.result_preview
        $duration = $resp.duration_ms

        if($status -eq "success") {
            Write-Host "  PASS ($duration ms)" -ForegroundColor Green
            if($preview -and $preview.Length -gt 0) {
                $short = $preview.Substring(0, [Math]::Min(200, $preview.Length))
                Write-Host "  Preview: $short"
            }
        } else {
            Write-Host "  FAIL ($duration ms): $errMsg" -ForegroundColor Red
            if($preview -and $preview.Length -gt 0) {
                $short = $preview.Substring(0, [Math]::Min(200, $preview.Length))
                Write-Host "  Preview: $short"
            }
        }
        return @{desc=$desc; status=$status; error=$errMsg; duration=$duration}
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
        return @{desc=$desc; status="error"; error="$($err.Message) $errDetail"; duration=0}
    }
}

$results = @()

# File tools
$results += Test-Tool "tool_read_file" '{"file_name":"README.md"}' "read_file"
$results += Test-Tool "tool_save_file" '{"path":"_test_tool_output.txt","content":"hello from tool test"}' "save_file"
$results += Test-Tool "tool_search_content" '{"query":"func main","path":"."}' "search_content"
$results += Test-Tool "tool_search_file" '{"pattern":"*.go","path":"."}' "search_file"
$results += Test-Tool "tool_read_multiple_files" '{"paths":["go.mod"]}' "read_multiple_files"
$results += Test-Tool "tool_list_file" '{"path":".","pattern":"*.md"}' "list_file"
$results += Test-Tool "tool_replace_content" '{"file_name":"_test_tool_output.txt","old_string":"hello","new_string":"world"}' "replace_content"
$results += Test-Tool "tool_diff_edit" '{"file_name":"_test_tool_output.txt","edits":[{"search":"world","replace":"hello"}]}' "diff_edit"
$results += Test-Tool "tool_patch_file" '{"file_name":"_test_tool_output.txt","patch":"--- a/_test_tool_output.txt\n+++ b/_test_tool_output.txt\n@@ -1 +1 @@\n-hello\n+patched"}' "patch_file"

# CLI admin tools
$results += Test-Tool "tool_cli_admin_agent_list" '{"limit":5}' "cli_admin_agent_list"
$results += Test-Tool "tool_cli_admin_skill_list" '{}' "cli_admin_skill_list"
$results += Test-Tool "tool_cli_admin_agent_get" '{"agent_id":"nonexistent"}' "cli_admin_agent_get"
$results += Test-Tool "tool_cli_admin_skill_get" '{"skill_id":"nonexistent"}' "cli_admin_skill_get"

# Other tools
$results += Test-Tool "tool_kanban" '{"action":"show"}' "kanban"
$results += Test-Tool "tool_browser" '{}' "browser"
$results += Test-Tool "tool_mcp_broker" '{"action":"list_servers"}' "mcp_broker"

Write-Host ""
Write-Host "====================================" -ForegroundColor Yellow
$pass = ($results | Where-Object { $_.status -eq "success" }).Count
$fail = ($results | Where-Object { $_.status -ne "success" }).Count
Write-Host "Results: $pass passed, $fail failed" -ForegroundColor Yellow
Write-Host "====================================" -ForegroundColor Yellow

foreach($r in $results) {
    $color = if($r.status -eq "success"){"Green"}else{"Red"}
    $errStr = if($r.error){" | $($r.error.Substring(0, [Math]::Min(100, $r.error.Length)))"}else{""}
    Write-Host "$($r.status.ToUpper()) | $($r.desc)$errStr" -ForegroundColor $color
}
