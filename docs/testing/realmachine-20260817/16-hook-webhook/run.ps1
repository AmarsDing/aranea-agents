# 16-hook-webhook: list/create/get/patch/delete + deliveries
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "16"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/hooks?page=1&page_size=20" -OutFile (Join-Path $ev "hk01-list.json")
$hcount = 0
try { $hcount = @($r.Body.items).Count } catch {}
Record $M "HK-01" "hook list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) hooks=$hcount" $r.Ms

# HK-02: SSRF guard must reject private webhook URL
$badBody = @{ key = "realmachine-test-hook-ssrf"; name = "ssrf-probe"; enabled = $false; config_json = '{"callback_point":"after_agent","action":{"type":"notify","webhook_url":"http://127.0.0.1:9/blackhole"}}' }
$r = Api-Post "/v1/hooks" $badBody -OutFile (Join-Path $ev "hk02-ssrf.json")
Record $M "HK-02" "SSRF guard rejects private webhook" ($(if ($r.Code -eq "400") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# HK-02B: create test hook with public URL
$body = @{ key = "realmachine-test-hook"; name = "realmachine test hook"; enabled = $false; config_json = '{"callback_point":"after_agent","action":{"type":"notify","webhook_url":"https://example.com/hook"}}' }
$r = Api-Post "/v1/hooks" $body -OutFile (Join-Path $ev "hk02b-create.json")
$hid = $null
try { $hid = $r.Body.id } catch {}
Record $M "HK-02B" "hook create (public url)" ($(if (($r.Code -eq "200" -or $r.Code -eq "201") -and $hid) { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$hid" $r.Ms

if ($hid) {
    $r = Api-Get "/v1/hooks/$hid" -OutFile (Join-Path $ev "hk03-get.json")
    Record $M "HK-03" "hook get" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    $r = Api-Patch "/v1/hooks/$hid" @{ name = "realmachine-test-hook-v2" } 
    Record $M "HK-04" "hook patch" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    $r = Api-Delete "/v1/hooks/$hid"
    Record $M "HK-05" "hook delete" ($(if ($r.Code -eq "200" -or $r.Code -eq "204") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
} else {
    Record $M "HK-03" "hook get" "SKIP" "create failed" 0
    Record $M "HK-04" "hook patch" "SKIP" "create failed" 0
    Record $M "HK-05" "hook delete" "SKIP" "create failed" 0
}

$r = Api-Get "/v1/hooks/deliveries?page=1&page_size=10" -OutFile (Join-Path $ev "hk06-deliveries.json")
Record $M "HK-06" "hook deliveries" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
