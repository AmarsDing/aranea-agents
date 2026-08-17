. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
Write-Host "BaseUrl=[$($script:BaseUrl)]"
Write-Host "TokenFile=[$($script:TokenFile)]"
$tok = Get-Token
Write-Host "tok-len=$($tok.Length)"
$r = Api-Get "/v1/agents/eval_memory_probe"
Write-Host "code=[$($r.Code)] ms=$($r.Ms) rawlen=$($r.Raw.Length)"
if ($r.Body) { Write-Host "id=$($r.Body.id) intentPass=$($r.Body.settings.intentPassEnabled)" }
