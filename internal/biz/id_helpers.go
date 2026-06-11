package biz

import (
	"encoding/json"

	"github.com/google/uuid"
)

// newBizID returns a new short business-layer ID. It is the SINGLE source
// of truth for entity / relation / saga ID generation across the memory
// L4 graph. The previous slug-based generator (slugEntityName) was
// removed because:
//  1. It truncated UTF-8 / CJK names mid-codepoint and silently lost
//     information.
//  2. It conflated "display name" with "primary key", so the same
//     business entity could end up with multiple UUIDs after slug drift
//     and the (scope, type, name_normalized) UNIQUE constraint would
//     then race two writes for the same business key.
//  3. It violated the "ID is a stable business identifier" invariant
//     that cascade compensation relies on (compensation payloads embed
//     the original entity ID; if that ID is regenerated every slug
//     drift, the payload becomes useless).
//
// All callers MUST use newBizID() and resolve pre-existing IDs through
// GetEntityByScopeKey (i.e. read-then-write), not by re-deriving from
// the name. This eliminates the dual-source-of-truth that produced
// BUG-03 / BUG-07.
func newBizID() string {
	return uuid.NewString()
}

// jsonEscape returns a JSON-safe string for embedding inside a JSON
// template literal (e.g. `{"alias_of":"`+jsonEscape(name)+`"}`). It is
// only used in a handful of metadata strings; stdlib json keeps the
// dependency surface small. Empty input or a marshal failure yields ""
// — the surrounding JSON remains valid because the result is wrapped
// in quotes by the caller.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		return string(b[1 : len(b)-1])
	}
	return string(b)
}
