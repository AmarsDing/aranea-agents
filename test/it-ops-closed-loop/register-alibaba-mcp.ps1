# D3-b: register 3 Alibaba MCP servers + health check + tool discovery
$ErrorActionPreference = "Stop"
$base = "http://localhost:8000"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" }
$uvx = "C:\Users\Administrator\AppData\Local\Programs\Python\Python314\Scripts\uvx.exe"

function Invoke-Api($method, $path, $body = $null) {
    $params = @{ Uri = "$base$path"; Method = $method; Headers = $headers; TimeoutSec = 300 }
    if ($body) { $params.Body = [Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 10 -Compress)) }
    return Invoke-RestMethod @params
}
function Save-Evidence($name, $data) {
    $data | ConvertTo-Json -Depth 30 | Set-Content (Join-Path $outDir "$name.json") -Encoding UTF8
    Write-Output "[saved] $name.json"
}

$servers = @(
    @{
        key = "alibaba-cloud-ops"
        name = "Alibaba Cloud Ops (ECS/CloudMonitor/OOS/RDS)"
        description = "aliyun/alibaba-cloud-ops-mcp-server: ECS instance ops, CloudMonitor metrics, OOS command execution, OSS/VPC/RDS management. Credentials via user credential keys ALIBABA_CLOUD_ACCESS_KEY_ID / ALIBABA_CLOUD_ACCESS_KEY_SECRET."
        cfg = @{ transport = "stdio"; command = $uvx; args = @("--python", "3.12", "alibaba-cloud-ops-mcp-server@latest"); timeout_sec = 120 }
    },
    @{
        key = "aliyun-observability-sls"
        name = "Aliyun Observability (SLS/ARMS/CloudMonitor)"
        description = "aliyun alibabacloud-observability-mcp-server (PyPI: mcp-server-aliyun-observability): SLS log query (text-to-SQL, execute sql, list logstores/projects), ARMS trace/profile analysis, CloudMonitor metrics."
        cfg = @{ transport = "stdio"; command = $uvx; args = @("--python", "3.12", "--with", "python-dotenv", "mcp-server-aliyun-observability@latest"); timeout_sec = 120 }
    },
    @{
        key = "alibabacloud-rds-openapi"
        name = "Alibaba Cloud RDS OpenAPI"
        description = "aliyun/alibabacloud-rds-openapi-mcp-server v3.1.2 (pinned mcp==1.13.1 for fastmcp compat): RDS instance management via OpenAPI - describe/restart instances, slow query logs, performance metrics."
        cfg = @{ transport = "stdio"; command = $uvx; args = @("--python", "3.12", "--with", "mcp==1.13.1", "alibabacloud-rds-openapi-mcp-server@3.1.2"); timeout_sec = 120 }
    }
)

$registered = @()
foreach ($s in $servers) {
    Write-Output ("== create " + $s.key + " ==")
    try {
        $resp = Invoke-Api Post "/v1/mcp-servers" @{
            key = $s.key; name = $s.name; description = $s.description
            enabled = $true; sort_order = 100
            config_json = ($s.cfg | ConvertTo-Json -Compress)
        }
        Save-Evidence ("mcp-create-" + $s.key) $resp
        $registered += @{ key = $s.key; id = $resp.id }
        Write-Output ("   created id=" + $resp.id)
    } catch {
        Write-Output ("   create failed: " + $_.Exception.Message)
        if ($_.ErrorDetails) { Write-Output $_.ErrorDetails.Message }
        # already exists? list and reuse
        $list = Invoke-Api Get "/v1/mcp-servers"
        $found = $list.items | Where-Object { $_.key -eq $s.key }
        if ($found) { $registered += @{ key = $s.key; id = $found.id }; Write-Output ("   reuse existing id=" + $found.id) }
    }
}

# health check + tool discovery via TestMCPServer
foreach ($r in $registered) {
    Write-Output ("== test " + $r.key + " (id=" + $r.id + ") ==")
    try {
        $t = Invoke-Api Post ("/v1/mcp-servers/" + $r.id + "/test") @{}
        Save-Evidence ("mcp-test-" + $r.key) $t
        Write-Output ("   ok=" + $t.ok + " status=" + $t.status + " message=" + $t.message)
    } catch {
        Write-Output ("   test failed: " + $_.Exception.Message)
        if ($_.ErrorDetails) { Write-Output $_.ErrorDetails.Message }
    }
}
Write-Output "== DONE =="
