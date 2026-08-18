# Aranea eval D15 verification (S1/S2/S3/S4/S5)
$tok = Get-Content "f:\myproject\aranea-agents\docker\.test-token.txt" -Raw
$tok = $tok.Trim()
$sid = '3c596474-a471-4fcf-ac49-ebdd8426f237'
$content = '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机SW-Eval-01的管理IP为10.20.99.1"，tags=["评测-核心交换机","SW-Eval-01"]，fact_id="eval-sw-ip"，confidence=0.95。只调用这一个工具。'
$body = @{
    session_id = $sid
    agent_key  = "eval_memory_probe"
    content    = $content
}
$bodyJson = $body | ConvertTo-Json -Depth 10 -Compress
$tmpFile = [IO.Path]::GetTempFileName()
[IO.File]::WriteAllText($tmpFile, $bodyJson, [Text.UTF8Encoding]::new($false))
$t0 = Get-Date
$respFile = [IO.Path]::GetTempFileName()
$code = (& curl.exe -s -o $respFile -w "%{http_code}" -m 180 -X POST -H "Authorization: Bearer $tok" -H "Content-Type: application/json" --data-binary "@$tmpFile" "http://127.0.0.1:8810/v1/chat/messages" 2>$null | Out-String).Trim()
$elapsed = [int]((Get-Date) - $t0).TotalMilliseconds
$bodyText = [IO.File]::ReadAllText($respFile, [Text.UTF8Encoding]::new($false))
Remove-Item $tmpFile, $respFile -Force
[IO.File]::WriteAllText("f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_test-d15-response.json", $bodyText, [Text.UTF8Encoding]::new($false))
Write-Host "elapsed=${elapsed}ms code=$code"
Write-Host "response_body_length=$($bodyText.Length)"
$bodyText | ConvertFrom-Json | ConvertTo-Json -Depth 10
