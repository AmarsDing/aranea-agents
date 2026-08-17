# 00-env-readiness 真机测试
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "00"
$ev = Join-Path $PSScriptRoot "evidence"

# ENV-01 aranea HTTP 健康
$r = Api-Get "/healthz" -TimeoutSec 10
Record $M "ENV-01" "aranea /healthz" ($(if ($r.Code -eq "200" -and $r.Body.status -eq "ok") { "PASS" } else { "FAIL" })) "code=$($r.Code) auth=$($r.Body.auth_mode)" $r.Ms

# ENV-02 gRPC 端口可达
$c = New-Object Net.Sockets.TcpClient
$sw = [Diagnostics.Stopwatch]::StartNew()
try { $c.Connect("localhost", 9910); $ok = $c.Connected } catch { $ok = $false }
$sw.Stop(); $c.Close()
Record $M "ENV-02" "gRPC :9910 TCP 可达" ($(if ($ok) { "PASS" } else { "FAIL" })) "" $sw.ElapsedMilliseconds

# ENV-03 WS 端口可达
$c = New-Object Net.Sockets.TcpClient
$sw = [Diagnostics.Stopwatch]::StartNew()
try { $c.Connect("localhost", 8812); $ok = $c.Connected } catch { $ok = $false }
$sw.Stop(); $c.Close()
Record $M "ENV-03" "WS :8812 TCP 可达" ($(if ($ok) { "PASS" } else { "FAIL" })) "" $sw.ElapsedMilliseconds

# ENV-04 PG 迁移表检查
$tbl = (docker exec aranea-postgres psql -U postgres -d aranea -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';" | Out-String).Trim()
$mig = (docker exec aranea-postgres psql -U postgres -d aranea -t -c "SELECT count(*) FROM schema_migrations;" 2>$null | Out-String).Trim()
Record $M "ENV-04" "PG 库表/迁移" ($(if ([int]$tbl.Trim() -gt 50) { "PASS" } else { "FAIL" })) "tables=$($tbl.Trim()) migrations=$($mig.Trim())"

# ENV-05 Redis
$ping = docker exec aranea-redis redis-cli ping
Record $M "ENV-05" "aranea-redis ping" ($(if ($ping -match "PONG") { "PASS" } else { "FAIL" })) "$ping"

# ENV-06 TwinMonitor gateway
$gw = curl.exe -s -o NUL -w "%{http_code}" -m 8 http://localhost:8000/healthz
Record $M "ENV-06" "TwinMonitor gateway :8000" ($(if ($gw -eq "200") { "PASS" } else { "FAIL" })) "code=$gw"

# ENV-07 GNS3 agent
$g = curl.exe -s -o NUL -w "%{http_code}" -m 8 http://localhost:18081/
$l = (netstat -ano | Select-String ":18081.*LISTENING" | Select-Object -First 1) -ne $null
Record $M "ENV-07" "GNS3 agent :18081 监听" ($(if ($l) { "PASS" } else { "FAIL" })) "root_code=$g"

# ENV-08 登录
$tok = Renew-Token
Record $M "ENV-08" "dev/dev 登录签发 JWT" ($(if ($tok.Length -gt 100) { "PASS" } else { "FAIL" })) "token_len=$($tok.Length)"

# ENV-09 admin 日志致命错误扫描
docker logs aranea-admin --since 24h 2>&1 | Select-String -Pattern "panic|FATAL|fatal" | Select-Object -First 20 | Out-File (Join-Path $ev "admin-fatals.txt")
$fatalCount = (Get-Content (Join-Path $ev "admin-fatals.txt") -ErrorAction SilentlyContinue | Measure-Object -Line).Lines
Record $M "ENV-09" "admin 24h 日志 panic/fatal" ($(if ($fatalCount -eq 0) { "PASS" } else { "FAIL" })) "hits=$fatalCount"

# ENV-10 skills 卷挂载
$mnt = docker inspect aranea-admin --format "{{range .Mounts}}{{.Destination}};{{end}}"
Record $M "ENV-10" "skills 卷挂载" ($(if ($mnt -match "Aranea/skills") { "PASS" } else { "FAIL" })) "$mnt"

# ENV-11 系统信息
$r = Api-Get "/v1/system/info" -TimeoutSec 10
$snip = if ($r.Raw) { $r.Raw.Substring(0, [Math]::Min(120, $r.Raw.Length)) } else { "" }
Record $M "ENV-11" "系统信息接口" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) ("code=" + $r.Code + " " + $snip) $r.Ms
