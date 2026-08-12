#!/usr/bin/env bash
# Agent Memory Challenge 2026 — Add/Search smoke self-test.
# Usage: ./test/agent-memory-challenge/smoke.sh [BASE_URL] [MEMORY_SYSTEM_KEY]
# Exits non-zero on the first failed check; prints a PASS report otherwise.
set -euo pipefail

BASE="${1:-http://localhost:8910}"
TOKEN="${2:-}"
AUTH=()
if [ -n "$TOKEN" ]; then AUTH=(-H "Authorization: Bearer ${TOKEN}"); fi
JSON=(-H 'Content-Type: application/json')

say()  { printf '%s\n' "$*"; }
fail() { printf 'SMOKE FAIL: %s\n' "$*" >&2; exit 1; }

say "== memoryeval smoke @ ${BASE}"

# 1. health
curl -sf "${BASE}/healthz" | grep -q '"ok"' || fail "GET /healthz"
say "ok  healthz"

# 2. auth negative check (only when a token is configured)
if [ -n "$TOKEN" ]; then
  CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${BASE}/v1/memory/search" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer wrong-${TOKEN}" \
    -d '{"user_id":"smoke-a","query":"x"}')
  [ "$CODE" = "401" ] || fail "auth negative check: got ${CODE}, want 401"
  say "ok  auth rejects wrong token (401)"
fi

# 3. validation negative checks
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${BASE}/v1/memory/search" \
  "${AUTH[@]}" "${JSON[@]}" -d '{"query":"missing user"}')
[ "$CODE" = "400" ] || fail "search without user_id: got ${CODE}, want 400"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${BASE}/v1/memory/add" \
  "${AUTH[@]}" "${JSON[@]}" -d '{"messages":[{"role":"user","content":"hi"}]}')
[ "$CODE" = "400" ] || fail "add without user_id: got ${CODE}, want 400"
say "ok  validation 400s"

# 4. add two users with conflicting facts (isolation probe)
curl -sf -X POST "${BASE}/v1/memory/add" "${AUTH[@]}" "${JSON[@]}" -d '{
  "request_id":"smoke-a1","user_id":"smoke-user-a","session_id":"s-a",
  "messages":[
    {"role":"user","content":"我最喜欢的编程语言是 Go"},
    {"role":"user","content":"我养了一只叫年糕的猫"}
  ]}' | grep -q '"success":true' || fail "add user A"
curl -sf -X POST "${BASE}/v1/memory/add" "${AUTH[@]}" "${JSON[@]}" -d '{
  "request_id":"smoke-b1","user_id":"smoke-user-b","session_id":"s-b",
  "messages":[{"role":"user","content":"我最喜欢的编程语言是 Rust"}]
}' | grep -q '"success":true' || fail "add user B"
say "ok  add user A / user B"

# 5. idempotent re-add (platform retry safety): same payload again must succeed
curl -sf -X POST "${BASE}/v1/memory/add" "${AUTH[@]}" "${JSON[@]}" -d '{
  "request_id":"smoke-a1-retry","user_id":"smoke-user-a","session_id":"s-a",
  "messages":[{"role":"user","content":"我最喜欢的编程语言是 Go"}]
}' | grep -q '"success":true' || fail "idempotent re-add"
say "ok  idempotent re-add"

# 6. search A: must recall Go fact, must NOT leak Rust fact
RESP=$(curl -sf -X POST "${BASE}/v1/memory/search" "${AUTH[@]}" "${JSON[@]}" \
  -d '{"user_id":"smoke-user-a","query":"编程语言","top_k":10}')
echo "${RESP}" | grep -q '"data"' || fail "search A: missing data field"
echo "${RESP}" | grep -q 'Go'   || fail "search A: expected Go fact not recalled: ${RESP}"
if echo "${RESP}" | grep -q 'Rust'; then
  fail "isolation breached: user B (Rust) fact leaked into user A results"
fi
say "ok  recall + user isolation"

# 7. search empty-scope user: data must be [] (not null, not error)
RESP=$(curl -sf -X POST "${BASE}/v1/memory/search" "${AUTH[@]}" "${JSON[@]}" \
  -d '{"user_id":"smoke-user-empty","query":"编程语言","top_k":10}')
echo "${RESP}" | grep -q '"data":\[\]' || fail "empty scope must return data:[]: ${RESP}"
say "ok  empty scope returns []"

say "SMOKE PASS"
