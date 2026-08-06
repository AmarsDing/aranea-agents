# D3-b step 2: point platform MCP configs at the persistent exe installs, re-test
$ErrorActionPreference = "Stop"
$base = "http://localhost:8000"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" }
$bin = "C:\Users\Administrator\.local\bin"

function Invoke-Api($method, $path, $body = $null) {
    $params = @{ Uri = "$base$path"; Method = $method; Headers = $headers; TimeoutSec = 300 }
    if ($body) { $params.Body = [Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 10 -Compress)) }
    return Invoke-RestMethod @params
}
function Save-Evidence($name, $data) {
    $data | ConvertTo-Json -Depth 30 | Set-Content (Join-Path $outDir "$name.json") -Encoding UTF8
    Write-Output "[saved] $name.json"
}

$updates = @(
    @{ id = "14aabb0a62a9d3794b90ad75"; key = "alibaba-cloud-ops";
       cfg = @{ transport = "stdio"; command = "$bin\alibaba-cloud-ops-mcp-server.exe"; args = @(); timeout_sec = 60 } },
    @{ id = "55643119f9efe0207c40c52f"; key = "aliyun-observability-sls";
       cfg = @{ transport = "stdio"; command = "$bin\mcp-server-aliyun-observability.exe"; args = @("--transport", "stdio"); timeout_sec = 60 } },
    @{ id = "1f6ddc7211320f0b43784214"; key = "alibabacloud-rds-openapi";
       cfg = @{ transport = "stdio"; command = "$bin\alibabacloud-rds-openapi-mcp-server.exe"; args = @(); timeout_sec = 60 } }
)

foreach ($u in $updates) {
    Write-Output ("== update " + $u.key + " ==")
    try {
        $resp = Invoke-Api Patch ("/v1/mcp-servers/" + $u.id) @{
            enabled = $true
            status = "active"
            config_json = ($u.cfg | ConvertTo-Json -Compress)
        }
        Write-Output "   updated"
    } catch {
        Write-Output ("   update failed: " + $_.Exception.Message)
        if ($_.ErrorDetails) { Write-Output $_.ErrorDetails.Message }
    }
    try {
        $t = Invoke-Api Post ("/v1/mcp-servers/" + $u.id + "/test") @{}
        Save-Evidence ("mcp-test-final-" + $u.key) $t
        Write-Output ("   test ok=" + $t.ok + " status=" + $t.status)
    } catch {
        Write-Output ("   test failed: " + $_.Exception.Message)
    }
}

$list = Invoke-Api Get "/v1/mcp-servers"
Save-Evidence "mcp-server-list-final" $list
Write-Output ("total mcp servers: " + @($list.items).Count)
foreach ($i in $list.items) { Write-Output (" - " + $i.key + " | " + $i.name + " | enabled=" + $i.enabled) }
Write-Output "== DONE =="
