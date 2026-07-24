$env:KRATOS_AUTH_SECRET = "aranea-portable-dev-secret-32chars!!"
$env:DEPLOY_ENV = "dev"
$env:KRATOS_HTTP_AUTH_DISABLED = "1"
$env:FEATURES_LOCAL_REVEAL_ENABLED = "1"
$outFile = "F:\aranea-agents\_temp\admin_stdout.log"
$errFile = "F:\aranea-agents\_temp\admin_stderr.log"
$proc = Start-Process -FilePath "F:\aranea-agents\bin\admin.exe" -ArgumentList "-conf","F:\aranea-agents\configs" -WorkingDirectory "F:\aranea-agents" -RedirectStandardOutput $outFile -RedirectStandardError $errFile -PassThru -WindowStyle Hidden
Write-Host "PID: $($proc.Id)"
Start-Sleep -Seconds 8
if ($proc.HasExited) {
    Write-Host "EXITED, code: $($proc.ExitCode)"
} else {
    Write-Host "RUNNING"
}
