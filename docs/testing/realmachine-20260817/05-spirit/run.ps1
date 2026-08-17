# 05-spirit dynamic orchestration real-machine test
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "05"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# SPIRIT-01 spirit agent detail
$r = Api-Get "/v1/agents/agent___spirit__" -OutFile (Join-Path $ev "spirit01-agent.json")
Record $M "SPIRIT-01" "spirit agent detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) key=$($r.Body.agentKey)" $r.Ms

# SPIRIT-02 chat options
$r = Api-Get "/v1/chat/options" -OutFile (Join-Path $ev "spirit02-options.json")
Record $M "SPIRIT-02" "chat options" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# SPIRIT-03 create session + send trivial message (real LLM, single call)
$r = Api-Post "/v1/sessions" @{ agent_id = "agent___spirit__"; title = "realmachine-spirit"; owner_type = "agent" } -OutFile (Join-Path $ev "spirit03-session.json")
$sid = $r.Body.id
Record $M "SPIRIT-03" "create spirit session" ($(if ($r.Code -eq "200" -and $sid) { "PASS" } else { "FAIL" })) "code=$($r.Code) sid=$sid" $r.Ms

if ($sid) {
    $r = Api-Post "/v1/chat/messages" @{ session_id = $sid; agent_key = "__spirit__"; content = "Reply with exactly: PONG. Do not create any plan, team or orchestration." } -OutFile (Join-Path $ev "spirit04-send.json") -TimeoutSec 240
    $has = ($null -ne $r.Body.agent_message)
    Record $M "SPIRIT-04" "spirit send msg -> LLM reply" ($(if ($r.Code -eq "200" -and $has) { "PASS" } else { "FAIL" })) "code=$($r.Code) reply=$has" $r.Ms

    # SPIRIT-05 teams of spirit session
    $r = Api-Get "/v1/spirit/$sid/teams" -OutFile (Join-Path $ev "spirit05-teams.json")
    Record $M "SPIRIT-05" "spirit teams list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    # SPIRIT-06 task plans
    $r = Api-Get "/v1/chat/plans?session_id=$sid" -OutFile (Join-Path $ev "spirit06-plans.json")
    Record $M "SPIRIT-06" "spirit task plans" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    # SPIRIT-07 synthesize (no teams expected -> observe behavior)
    $r = Api-Post "/v1/spirit/$sid/synthesize" @{ strategy = "template" } -OutFile (Join-Path $ev "spirit07-synthesize.json")
    Record $M "SPIRIT-07" "synthesize (empty teams)" ($(if ($r.Code -in @("200","400","404","409")) { "PASS" } else { "FAIL" })) "code=$($r.Code) msg=$($r.Body.message)" $r.Ms
}
