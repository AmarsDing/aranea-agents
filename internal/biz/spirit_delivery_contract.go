package biz

import (
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"

	"sort"
	"strings"
)

// resolveDeliverableKeyContent resolves one payload key with the same source
// priority as resolveDeliverableFullContent: graph-state re-read first, then
// the persisted envelope. available lists the readable payload keys when a
// source was reachable but the key missing (LLM-actionable NotFound); found
// reports the key existed in a reachable source (its rendered content may
// still be empty — the caller reports that honestly instead of a misleading
// "not found").
func (d *SpiritDelivery) resolveDeliverableKeyContent(ctx context.Context, t Team, key string) (teamFullOutput, []string, bool, error) {
	if t.DagNodeID != "" {
		if out, available, found, ok := d.readGraphStateKeyContent(ctx, t, key); ok {
			return out, available, found, nil
		}
		if out, available, found, ok := d.readEnvelopeKeyContent(t, key); ok {
			return out, available, found, nil
		}
	}
	return teamFullOutput{}, nil, false, nil
}

// readGraphStateKeyContent reads one payload key from the graph final state.
// ok=false means the source is unreadable (caller degrades to the envelope);
// ok=true with found=false means the key is absent — available then lists
// the payload keys present.
func (d *SpiritDelivery) readGraphStateKeyContent(ctx context.Context, t Team, key string) (teamFullOutput, []string, bool, bool) {
	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return teamFullOutput{}, nil, false, false
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil || teamSessionID == "" {
		return teamFullOutput{}, nil, false, false
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil || len(stateDeliv) == 0 {
		return teamFullOutput{}, nil, false, false
	}
	v, exists := stateDeliv[key]
	if !exists {
		return teamFullOutput{}, payloadStateKeys(stateDeliv), false, true
	}
	return teamFullOutput{Content: renderPayloadContent(v), SessionID: teamSessionID}, nil, true, true
}

// readEnvelopeKeyContent reads one payload key from the persisted envelope's
// StructuredJSON (same ok/found/available semantics as the graph-state variant).
func (d *SpiritDelivery) readEnvelopeKeyContent(t Team, key string) (teamFullOutput, []string, bool, bool) {
	ref, ok := d.readDeliverableRef(t)
	if !ok || strings.TrimSpace(ref.StructuredJSON) == "" {
		return teamFullOutput{}, nil, false, false
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(ref.StructuredJSON), &structured); err != nil || len(structured) == 0 {
		return teamFullOutput{}, nil, false, false
	}
	v, exists := structured[key]
	if !exists {
		keys := make([]string, 0, len(structured))
		for k := range structured {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return teamFullOutput{}, keys, false, true
	}
	return teamFullOutput{Content: renderPayloadContent(v), SessionID: ref.TeamSessionID}, nil, true, true
}

// resolveDeliverableFullContent returns the full deliverable content of a
// completed team, sourced from the DELIVERABLE itself — never the reply
// (2026-07-25 Fix 7). Priority:
//  1. graph-state re-read: untruncated, and the same source the envelope was
//     persisted from, so the full text can never disagree with the injected
//     summary (the envelope summary is truncated at MaxSummaryLen — only a
//     state re-read fulfills the tool's "full text" promise);
//  2. persisted DeliverableRef envelope, when the graph session is
//     unreadable (StructuredJSON is untruncated; Summary may be);
//  3. legacy reply extraction, for non-DAG teams and rows completed before
//     the Fix-1 gate (no envelope can exist for those).
func (d *SpiritDelivery) resolveDeliverableFullContent(ctx context.Context, t Team) (teamFullOutput, error) {
	if t.DagNodeID != "" {
		if full, ok := d.readGraphStateFullContent(ctx, t); ok {
			return full, nil
		}
		if full, ok := d.readEnvelopeFullContent(t); ok {
			return full, nil
		}
	}
	return d.extractTeamFullOutput(ctx, t.ID)
}

// readGraphStateFullContent renders the full deliverable from the graph final
// state: the reserved "summary" key (untruncated, unlike the envelope's) plus
// the non-reserved keys as a JSON object. ok=false on any gap (channel
// disabled, no session, read error, empty state) — the caller then degrades
// to the persisted envelope.
func (d *SpiritDelivery) readGraphStateFullContent(ctx context.Context, t Team) (teamFullOutput, bool) {
	anchor, ok := d.stateDeliverableChannel(t)
	if !ok {
		return teamFullOutput{}, false
	}
	teamSessionID, err := d.resolveTeamMainSessionID(ctx, t.ID)
	if err != nil || teamSessionID == "" {
		return teamFullOutput{}, false
	}
	stateDeliv, err := d.graphDelivReader.ReadGraphDeliverable(ctx, anchor, ctxuser.TRPCUserKey(ctx), teamSessionID)
	if err != nil || len(stateDeliv) == 0 {
		return teamFullOutput{}, false
	}
	summary, _ := stateDeliv[deliverableReservedKeySummary].(string)
	content := strings.TrimSpace(summary)
	if structured := marshalNonReservedStateKeys(stateDeliv); structured != "" {
		if content != "" {
			content += "\n\n"
		}
		content += structured
	}
	if content == "" {
		return teamFullOutput{}, false
	}
	return teamFullOutput{Content: content, SessionID: teamSessionID}, true
}

// readEnvelopeFullContent renders full content from the persisted
// DeliverableRef envelope: the untruncated StructuredJSON, preceded by the
// Summary when present. ok=false when no envelope exists for the team's node
// or the envelope carries nothing.
func (d *SpiritDelivery) readEnvelopeFullContent(t Team) (teamFullOutput, bool) {
	ref, ok := d.readDeliverableRef(t)
	if !ok {
		return teamFullOutput{}, false
	}
	content := strings.TrimSpace(ref.Summary)
	if structured := strings.TrimSpace(ref.StructuredJSON); structured != "" {
		if content != "" {
			content += "\n\n"
		}
		content += structured
	}
	if content == "" {
		return teamFullOutput{}, false
	}
	return teamFullOutput{Content: content, SessionID: ref.TeamSessionID}, true
}

// validateUpstreamContract performs the tool-call level contract check for
// ReadUpstreamDeliverable: reader team (resolved from readerSessionID) → its
// InputContract vs the upstream team's declared Deliverables. A mismatch
// returns *ContractMismatchError; the check is skipped (nil) when the reader
// side is unresolvable or either side declares no contract — declarations
// stay advisory for legacy teams without contracts.
// Domain: Delivery — runtime contract guard for upstream deliverable reads.
func (d *SpiritDelivery) validateUpstreamContract(ctx context.Context, readerSessionID string, upstream Team) error {
	reader, inputContracts, ok := d.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
	if !ok {
		return nil
	}
	upstreamContracts, err := ParseDeliverableContracts(upstream.Deliverables)
	if err != nil || len(upstreamContracts) == 0 {
		return nil
	}
	validator := d.contractValidator
	if validator == nil {
		validator = NewDeliverableContractValidator()
	}
	mismatches := validator.ValidateContractMatchDetailed(upstreamContracts, inputContracts)
	if len(mismatches) == 0 {
		return nil
	}
	d.lg.Warn("上游交付物契约校验不匹配",
		loggateway.StepID("spirit.read_upstream_deliverable.contract_mismatch"),
		loggateway.Str("reader_team_id", reader.ID),
		loggateway.Str("upstream_team_id", upstream.ID),
		loggateway.Int("mismatch_count", len(mismatches)),
	)
	return &ContractMismatchError{ReaderTeamID: reader.ID, UpstreamTeamID: upstream.ID, Mismatches: mismatches}
}

// resolveReaderContracts resolves the reader team and its parsed input
// contract for the runtime contract guards. ok=false covers every
// advisory-skip case: empty session id, unresolvable session/team, a team
// reading its own deliverable, or no declared input contract.
func (d *SpiritDelivery) resolveReaderContracts(ctx context.Context, readerSessionID, upstreamTeamID string) (Team, []DeliverableContract, bool) {
	readerSessionID = strings.TrimSpace(readerSessionID)
	if readerSessionID == "" {
		return Team{}, nil, false
	}
	sess, err := d.sessionUC.Get(ctx, readerSessionID)
	if err != nil || sess.TeamID == "" || sess.TeamID == upstreamTeamID {

		return Team{}, nil, false
	}
	reader, err := d.teamUC.Get(ctx, sess.TeamID)
	if err != nil {
		return Team{}, nil, false
	}
	inputContracts, err := ParseDeliverableContracts(reader.InputContract)
	if err != nil || len(inputContracts) == 0 {
		return Team{}, nil, false
	}
	return reader, inputContracts, true
}

// validateUpstreamContractSchema is the C2 content-level contract guard for
// ReadUpstreamDeliverable. For each reader input-contract entry carrying a
// schema_json AND declaring format=="json", the upstream deliverable content
// is validated against that schema. A violation joins a
// *ContractMismatchError (LLM-actionable, auto-correct-and-retry).
//
// Advisory skips (never block legacy teams): entry without schema_json,
// entry with non-json format, upstream side not declaring the same-named
// json deliverable, content that is not valid JSON, and schema execution
// errors (e.g. an invalid schema declaration on the reader side).
// Domain: Delivery — content-level schema guard for upstream deliverable reads.
func (d *SpiritDelivery) validateUpstreamContractSchema(ctx context.Context, readerSessionID string, upstream Team, content string) error {
	reader, inputContracts, ok := d.resolveReaderContracts(ctx, readerSessionID, upstream.ID)
	if !ok {
		return nil
	}
	needsSchema := false
	for _, c := range inputContracts {
		if strings.TrimSpace(c.SchemaJSON) != "" && c.Format == "json" {
			needsSchema = true
			break
		}
	}
	if !needsSchema {
		return nil
	}
	if !json.Valid([]byte(content)) {
		return nil
	}
	upstreamContracts, err := ParseDeliverableContracts(upstream.Deliverables)
	if err != nil || len(upstreamContracts) == 0 {
		return nil
	}
	upstreamJSONDeclared := make(map[string]bool, len(upstreamContracts))
	for _, c := range upstreamContracts {
		if c.Format == "json" {
			upstreamJSONDeclared[c.Name] = true
		}
	}

	var mismatches []ContractMismatch
	for _, entry := range inputContracts {
		if strings.TrimSpace(entry.SchemaJSON) == "" || entry.Format != "json" {
			continue
		}
		if !upstreamJSONDeclared[entry.Name] {
			continue
		}
		verr := shared.ValidateDocumentAgainstSchema("SPIRIT", entry.SchemaJSON, content)
		if verr == nil {
			continue
		}
		if !apierror.IsCode(verr, apierror.CodeBadRequest) {
			continue
		}
		detail := verr.Error()
		if ae, ok := apierror.From(verr); ok {
			detail = strings.TrimPrefix(ae.Message, schemaViolationPrefix)
		}
		mismatches = append(mismatches, ContractMismatch{
			Name:     entry.Name,
			Kind:     ContractMismatchSchema,
			Expected: detail,
		})
	}
	if len(mismatches) == 0 {
		return nil
	}
	d.lg.Warn("上游交付物内容 schema 校验不匹配",
		loggateway.StepID("spirit.read_upstream_deliverable.schema_mismatch"),
		loggateway.Str("reader_team_id", reader.ID),
		loggateway.Str("upstream_team_id", upstream.ID),
		loggateway.Int("mismatch_count", len(mismatches)),
	)
	return &ContractMismatchError{ReaderTeamID: reader.ID, UpstreamTeamID: upstream.ID, Mismatches: mismatches}
}
