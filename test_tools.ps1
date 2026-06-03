$tests = @(
    @{ name = "read_file"; id = "tool_read_file"; args = '{"file_name":"README.md"}' },
    @{ name = "read_multiple_files"; id = "tool_read_multiple_files"; args = '{"patterns":["go.mod"]}' },
    @{ name = "save_file"; id = "tool_save_file"; args = '{"file_name":"_test_tool.txt","contents":"test content","overwrite":true}' },
    @{ name = "list_file"; id = "tool_list_file"; args = '{}' },
    @{ name = "search_file"; id = "tool_search_file"; args = '{"pattern":"*.md"}' },
    @{ name = "search_content"; id = "tool_search_content"; args = '{"content_pattern":"Aranea","file_pattern":"*.md"}' },
    @{ name = "replace_content"; id = "tool_replace_content"; args = '{"file_name":"_test_tool.txt","old_string":"test","new_string":"TEST"}' },
    @{ name = "diff_edit"; id = "tool_diff_edit"; args = '{"file_name":"_test_tool.txt","edits":[{"search":"TEST","replace":"test"}]}' },
    @{ name = "patch_file"; id = "tool_patch_file"; args = '{"file_name":"_test_tool.txt","patch":"--- a/_test_tool.txt\n+++ b/_test_tool.txt\n@@ -1 +1 @@\n-test content\n+patched content\n"}' }
)

foreach ($t in $tests) {
    $body = @{ arguments_json = $t.args; timeout_sec = 30 } | ConvertTo-Json -Compress
    try {
        $r = Invoke-RestMethod -Uri "http://localhost:8000/v1/tools/$($t.id)/test" -Method POST -ContentType "application/json" -Body $body
        Write-Output "$($t.name): $($r.status) | err=$($r.errorMessage) | dur=$($r.durationMs)ms"
    } catch {
        $errMsg = $_.Exception.Message
        if ($_.Exception.Response) {
            $reader = [System.IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
            $errMsg = $reader.ReadToEnd()
            $reader.Close()
        }
        Write-Output "$($t.name): FAILED | $errMsg"
    }
}
