$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"; "Content-Type"="application/json"}

# Update save_file paramsSchema
$saveFileBody = '{"id":"tool_save_file","key":"save_file","display_name":"保存文件","parameters_schema_json":"{\"type\":\"object\",\"properties\":{\"file_name\":{\"type\":\"string\",\"description\":\"文件路径\"},\"contents\":{\"type\":\"string\",\"description\":\"文件内容\"},\"overwrite\":{\"type\":\"boolean\",\"description\":\"是否覆盖已有文件\"}},\"required\":[\"file_name\",\"contents\"]}"}'
Write-Host "Updating save_file..."
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_save_file" -Method Put -Headers $headers -Body $saveFileBody -TimeoutSec 10
    Write-Host "  OK: $($resp.key) paramsSchema updated"
} catch {
    Write-Host "  Error: $($_.Exception.Message)"
}

# Update read_multiple_files paramsSchema
$readMultiBody = '{"id":"tool_read_multiple_files","key":"read_multiple_files","display_name":"批量读取文件","parameters_schema_json":"{\"type\":\"object\",\"properties\":{\"patterns\":{\"type\":\"array\",\"items\":{\"type\":\"string\"},\"description\":\"Glob 模式列表，如 *.go 或 workspace://out/*.txt\"},\"case_sensitive\":{\"type\":\"boolean\",\"description\":\"Glob 匹配是否区分大小写\"}},\"required\":[\"patterns\"]}"}'
Write-Host "Updating read_multiple_files..."
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_read_multiple_files" -Method Put -Headers $headers -Body $readMultiBody -TimeoutSec 10
    Write-Host "  OK: $($resp.key) paramsSchema updated"
} catch {
    Write-Host "  Error: $($_.Exception.Message)"
}

# Update search_content paramsSchema
$searchContentBody = '{"id":"tool_search_content","key":"search_content","display_name":"内容搜索","parameters_schema_json":"{\"type\":\"object\",\"properties\":{\"content_pattern\":{\"type\":\"string\",\"description\":\"正则表达式搜索模式\"},\"path\":{\"type\":\"string\",\"description\":\"搜索根目录\"},\"file_pattern\":{\"type\":\"string\",\"description\":\"文件 glob 匹配模式\"},\"file_case_sensitive\":{\"type\":\"boolean\",\"description\":\"文件匹配是否区分大小写\"},\"content_case_sensitive\":{\"type\":\"boolean\",\"description\":\"内容匹配是否区分大小写\"}},\"required\":[\"content_pattern\"]}"}'
Write-Host "Updating search_content..."
try {
    $resp = Invoke-RestMethod -Uri "$baseUrl/v1/tools/tool_search_content" -Method Put -Headers $headers -Body $searchContentBody -TimeoutSec 10
    Write-Host "  OK: $($resp.key) paramsSchema updated"
} catch {
    Write-Host "  Error: $($_.Exception.Message)"
}
