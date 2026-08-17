# 09-mcp real-machine test
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "09"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# MCP-01 list servers
$r = Api-Get "/v1/mcp-servers" -OutFile (Join-Path $ev "mcp01-list.json")
$items = @($r.Body.items)
Record $M "MCP-01" "mcp servers list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$($items.Count)" $r.Ms

# MCP-02 server detail (first)
if ($items.Count -ge 1) {
    $mid = $items[0].id
    $r = Api-Get "/v1/mcp-servers/$mid" -OutFile (Join-Path $ev "mcp02-detail.json")
    Record $M "MCP-02" "server detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) key=$($r.Body.serverKey)" $r.Ms

    # MCP-03 test connection (live probe)
    $r = Api-Post "/v1/mcp-servers/$mid/test" @{} -OutFile (Join-Path $ev "mcp03-test.json") -TimeoutSec 90
    Record $M "MCP-03" "test connection ($($items[0].serverKey))" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

# MCP-04 validate endpoint (bad config should be rejected with clear reason)
$r = Api-Post "/v1/mcp-servers/validate" @{ name = "realmachine-validate"; transport = "stdio"; command = "nonexistent-binary-xyz" } -OutFile (Join-Path $ev "mcp04-validate.json")
Record $M "MCP-04" "validate (bad config)" ($(if ($r.Code -in @("200","400") ) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MCP-05 user credentials list
if ($items.Count -ge 1) {
    $r = Api-Get "/v1/mcp-servers/$mid/user-credentials" -OutFile (Join-Path $ev "mcp05-creds.json")
    Record $M "MCP-05" "user credentials list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}
