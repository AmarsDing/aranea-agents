// Package runctx carries the per-call RuntimeContext (tenant, user, session,
// agent, trace, budget) through context.Context. Driving adapters construct
// a RuntimeContext at the edge; downstream layers read it via runctx.From.
//
// See aranea/docs/0 main design.md §5.3 for the injection rules. The current
// RuntimeContext type lives in this package after row #1 migration; the
// context.Context From/With helpers will land in a subsequent row.
package runctx
