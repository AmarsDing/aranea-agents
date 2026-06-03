#!/usr/bin/env bash
# Pattern-based fix fallback for auto-fix engine.
# When the self-hosted Agent API is not available, this script attempts
# to match known failure patterns and apply corresponding fixes.
#
# Usage: pattern-fix.sh <failure-logs-file>

set -euo pipefail

LOGS="${1:-failure-logs.txt}"

if [ ! -f "$LOGS" ]; then
  echo "No failure logs file found: $LOGS"
  exit 0
fi

echo "Attempting pattern-based fixes..."

# Pattern 1: Race condition → add sync.RWMutex
if grep -qE "DATA RACE|concurrent map|race detected" "$LOGS"; then
  echo "[PATTERN] Race condition detected. See .auto-fix/known-fixes/race-condition.md for manual fix guidance."
  # Race conditions require manual inspection; cannot safely auto-fix.
fi

# Pattern 2: Nil pointer dereference → add nil check
if grep -qE "nil pointer dereference|invalid memory address" "$LOGS"; then
  echo "[PATTERN] Nil pointer dereference detected. See .auto-fix/known-fixes/nil-pointer.md for manual fix guidance."
  # Nil pointer fixes require understanding the specific code path.
fi

# Pattern 3: Import cycle → extract shared types
if grep -qE "import cycle not allowed" "$LOGS"; then
  echo "[PATTERN] Import cycle detected. See .auto-fix/known-fixes/import-cycle.md for manual fix guidance."
  # Import cycles require architectural decisions.
fi

# Pattern 4: Proto/wire out of sync → regenerate
if grep -qE "Proto generated files are out of date|wire_gen.go is out of date" "$LOGS"; then
  echo "[PATTERN] Generated files out of sync. Running make api && make wire..."
  make api 2>/dev/null && make wire 2>/dev/null || true
fi

# Pattern 5: gofmt drift → reformat
if grep -qE "gofmt|formatted differently" "$LOGS"; then
  echo "[PATTERN] Go formatting drift. Running gofmt..."
  gofmt -w . 2>/dev/null || true
fi

# Pattern 6: goimports drift → fix imports
if grep -qE "goimports|malformed import" "$LOGS"; then
  echo "[PATTERN] Import formatting drift. Running goimports..."
  command -v goimports >/dev/null 2>&1 && goimports -w . || true
fi

# Pattern 7: go mod tidy needed
if grep -qE "go: module|missing go.sum entry|go.sum is out of date" "$LOGS"; then
  echo "[PATTERN] Go module issue. Running go mod tidy..."
  go mod tidy 2>/dev/null || true
fi

echo "Pattern-based fix scan complete."
