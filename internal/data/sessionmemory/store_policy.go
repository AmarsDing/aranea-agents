package sessionmemory

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func (st *Store) SetPolicyEngine(engine *biz.MemoryPolicyEngine) {
	if st == nil {
		return
	}
	st.policy = engine
}

// WriteMemoryActionLog implements biz.MemoryActionLogWriter.
func (st *Store) WriteMemoryActionLog(ctx context.Context, rec biz.MemoryPolicyRecord) error {
	return st.InsertMemoryActionLog(ctx, MemoryActionLogInsert{
		Action:         rec.Action,
		TargetKind:     rec.TargetKind,
		TargetID:       rec.TargetID,
		Reason:         rec.Reason,
		PolicyVersion:  rec.PolicyVersion,
		TurnID:         rec.TurnID,
		SourceEventIDs: rec.SourceEventIDs,
		MetadataJSON:   rec.MetadataJSON,
	})
}

func (st *Store) recordPolicyBestEffort(ctx context.Context, in MemoryActionLogInsert) error {
	if st == nil {
		return nil
	}
	if st.policy != nil {
		return st.policy.RecordBestEffort(ctx, biz.MemoryPolicyRecord{
			Action:         in.Action,
			TargetKind:     in.TargetKind,
			TargetID:       in.TargetID,
			Reason:         in.Reason,
			PolicyVersion:  in.PolicyVersion,
			TurnID:         in.TurnID,
			SourceEventIDs: in.SourceEventIDs,
			MetadataJSON:   in.MetadataJSON,
		})
	}
	st.insertMemoryActionLogBestEffort(ctx, in)
	return nil
}

// recordPolicyOnTx writes an audit row inside an open transaction.
// Failures abort the batch only when strict policy mode is enabled (matches RecordBestEffort).
func (st *Store) recordPolicyOnTx(ctx context.Context, db sqlRunner, in MemoryActionLogInsert) error {
	if st == nil {
		return nil
	}
	action := strings.TrimSpace(in.Action)
	targetKind := strings.TrimSpace(in.TargetKind)
	targetID := strings.TrimSpace(in.TargetID)
	if action == "" || targetKind == "" || targetID == "" {
		return nil
	}
	if strings.TrimSpace(in.PolicyVersion) == "" {
		in.PolicyVersion = biz.PolicyVersionConsolidateV1
	}
	err := st.insertMemoryActionLogOn(ctx, db, in)
	if err == nil {
		return nil
	}
	if st.policy != nil && st.policy.StrictEnabled(ctx) {
		return err
	}
	return nil
}
