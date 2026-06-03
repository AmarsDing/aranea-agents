$ErrorActionPreference = "SilentlyContinue"
$h = @{"Authorization"="Bearer dev"; "Content-Type"="application/json"}

function Test-Tool {
    param([string]$ToolId, [string]$ArgsJson)
    $body = @{arguments_json=$ArgsJson; timeout_sec=15} | ConvertTo-Json -Compress
    try {
        $r = Invoke-RestMethod -Uri "http://localhost:8000/v1/tools/$ToolId/test" -Method POST -Headers $h -Body ([System.Text.Encoding]::UTF8.GetBytes($body)) -TimeoutSec 25
        return @{status=$r.status; duration=$r.durationMs; error=$r.errorMessage}
    } catch {
        $errResp = $_.Exception.Response
        if ($errResp) {
            try {
                $stream = $errResp.GetResponseStream()
                $reader = [System.IO.StreamReader]::new($stream)
                $errBody = $reader.ReadToEnd()
                $reader.Close()
                return @{status="HTTP_$([int]$errResp.StatusCode)"; duration=0; error=$errBody.Substring(0, [Math]::Min(200, $errBody.Length))}
            } catch {
                return @{status="ERROR"; duration=0; error=$_.Exception.Message.Substring(0, [Math]::Min(100, $_.Exception.Message.Length))}
            }
        }
        return @{status="ERROR"; duration=0; error=$_.Exception.Message.Substring(0, [Math]::Min(100, $_.Exception.Message.Length))}
    }
}

$sb = [System.Text.StringBuilder]::new()

# Test key tools
$tests = @(
    ,@("tool_read_file", '{"file_name":"go.mod"}')
    ,@("tool_list_file", '{}')
    ,@("tool_search_file", '{"pattern":"*.go"}')
    ,@("tool_save_file", '{"file_name":"_test_e2e.txt","content":"hello","overwrite":true}')
    ,@("tool_replace_content", '{"file_name":"_test_e2e.txt","old_string":"hello","new_string":"world"}')
    ,@("tool_read_file", '{"file_name":"_test_e2e.txt"}')
    ,@("tool_web_fetch", '{"url":"https://httpbin.org/get"}')
    ,@("tool_wikipedia_search", '{"query":"Go programming language"}')
    ,@("tool_await_user_reply", '{}')
    ,@("tool_shell_exec", '{"command":"echo hello"}')
    ,@("tool_todo_write", '{"todos":[{"id":"1","content":"test","status":"pending","priority":"medium","activeForm":"Testing"}]}')
    ,@("tool_working_memory_list", '{}')
    ,@("tool_working_memory_read", '{"field_path":"test"}')
    ,@("tool_read_document", '{"path":"go.mod"}')
    ,@("tool_read_spreadsheet", '{"path":"go.mod"}')
    ,@("tool_send_email", '{}')
    ,@("tool_claude_code", '{"command":"echo test"}')
)

foreach($t in $tests) {
    $r = Test-Tool $t[0] $t[1]
    $line = "$($t[0])|$($r.status)|$($r.duration)ms|err=$($r.error)"
    [void]$sb.AppendLine($line)
    Write-Host $line
}

[System.IO.File]::WriteAllText("F:\aranea-agents\tool_test_results.txt", $sb.ToString(), [System.Text.Encoding]::UTF8)
Write-Host "Done"
