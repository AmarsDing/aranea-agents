package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/decision/v1"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	structpb "google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// DecisionRecordService implements the proto-generated
// DecisionRecordServiceServer (M80 Phase 1 统一查询面，设计 §4.1/§5)。
type DecisionRecordService struct {
	v1.UnimplementedDecisionRecordServiceServer

	uc *decision.QueryUsecase
	lg loggateway.Logger
}

// NewDecisionRecordService is the Wire-friendly constructor.
func NewDecisionRecordService(uc *decision.QueryUsecase, lg loggateway.Logger) *DecisionRecordService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &DecisionRecordService{uc: uc, lg: lg.With(loggateway.Domain("decision_record_svc"))}
}

// ListDecisionRecords 分页查询决策记录（category/actor/entity/run/时间窗过滤）。
func (s *DecisionRecordService) ListDecisionRecords(ctx context.Context, req *v1.ListDecisionRecordsRequest) (*v1.ListDecisionRecordsResponse, error) {
	f := decision.ListFilter{
		Category:    req.GetCategory(),
		ActorKey:    req.GetActorKey(),
		EntityType:  req.GetEntityType(),
		EntityKey:   req.GetEntityKey(),
		SourceRunID: req.GetSourceRunId(),
		Page:        int(req.GetPage()),
		PageSize:    int(req.GetPageSize()),
	}
	if req.GetTimeFrom() != nil {
		f.TimeFrom = req.GetTimeFrom().AsTime()
	}
	if req.GetTimeTo() != nil {
		f.TimeTo = req.GetTimeTo().AsTime()
	}
	// 服务端先收敛分页（响应回填 page/page_size 用）；usecase 内幂等再收敛。
	f.NormalizePage()
	items, total, err := s.uc.List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := &v1.ListDecisionRecordsResponse{
		Items:    make([]*v1.DecisionRecord, 0, len(items)),
		Total:    total,
		Page:     int32(f.Page),
		PageSize: int32(f.PageSize),
	}
	for i := range items {
		msg, err := decisionRecordToProto(&items[i])
		if err != nil {
			return nil, apierror.Internal("DECISION", "record encode: %v", err)
		}
		out.Items = append(out.Items, msg)
	}
	return out, nil
}

// GetDecisionRecord 按 decision_key 查询单条；未命中返回 NotFound。
func (s *DecisionRecordService) GetDecisionRecord(ctx context.Context, req *v1.GetDecisionRecordRequest) (*v1.GetDecisionRecordResponse, error) {
	rec, err := s.uc.Get(ctx, req.GetDecisionKey())
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, apierror.NotFound("DECISION", "decision %s not found", req.GetDecisionKey())
	}
	msg, err := decisionRecordToProto(rec)
	if err != nil {
		return nil, apierror.Internal("DECISION", "record encode: %v", err)
	}
	return &v1.GetDecisionRecordResponse{Record: msg}, nil
}

// GetDecisionChain 追溯决策链（M80 1.8，设计 §5）：root + upstream（[0]=
// 直接父，虚拟父带 virtual_parent=true）+ downstream（深度升序）。
func (s *DecisionRecordService) GetDecisionChain(ctx context.Context, req *v1.GetDecisionChainRequest) (*v1.GetDecisionChainResponse, error) {
	chain, err := s.uc.GetChain(ctx, req.GetDecisionKey(), req.GetDirection(), int(req.GetMaxDepth()))
	if err != nil {
		return nil, err
	}
	if chain == nil {
		return nil, apierror.NotFound("DECISION", "decision %s not found", req.GetDecisionKey())
	}
	out := &v1.GetDecisionChainResponse{}
	if out.Root, err = decisionRecordToProto(chain.Root); err != nil {
		return nil, apierror.Internal("DECISION", "record encode: %v", err)
	}
	encode := func(recs []decision.Record) ([]*v1.DecisionRecord, error) {
		msgs := make([]*v1.DecisionRecord, 0, len(recs))
		for i := range recs {
			msg, err := decisionRecordToProto(&recs[i])
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msg)
		}
		return msgs, nil
	}
	if out.Upstream, err = encode(chain.Upstream); err != nil {
		return nil, apierror.Internal("DECISION", "record encode: %v", err)
	}
	if out.Downstream, err = encode(chain.Downstream); err != nil {
		return nil, apierror.Internal("DECISION", "record encode: %v", err)
	}
	return out, nil
}

// decisionRecordToProto maps the biz Record onto the wire message
// （entities 用 ListValue，source_ref/metadata 用 Struct）。
func decisionRecordToProto(rec *decision.Record) (*v1.DecisionRecord, error) {
	msg := &v1.DecisionRecord{
		Id:            rec.ID,
		DecisionKey:   rec.DecisionKey,
		Category:      string(rec.Category),
		Scenario:      rec.Scenario,
		Reasoning:     rec.Reasoning,
		Outcome:       rec.Outcome,
		ActorType:     string(rec.ActorType),
		ActorKey:      rec.ActorKey,
		WorkspaceId:   rec.WorkspaceID,
		VirtualParent: rec.VirtualParent,
	}
	if rec.Confidence != nil {
		msg.Confidence = rec.Confidence
	}
	if rec.ParentDecisionID != nil {
		msg.ParentDecisionId = rec.ParentDecisionID
	}
	if len(rec.RelatedEntities) > 0 {
		vals := make([]*structpb.Value, 0, len(rec.RelatedEntities))
		for _, e := range rec.RelatedEntities {
			v, err := structpb.NewValue(map[string]any{"type": e.Type, "key": e.Key})
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
		msg.RelatedEntities = &structpb.ListValue{Values: vals}
	}
	src := map[string]any{}
	putNonEmpty := func(k, v string) {
		if v != "" {
			src[k] = v
		}
	}
	putNonEmpty("run_id", rec.SourceRef.RunID)
	putNonEmpty("step_id", rec.SourceRef.StepID)
	putNonEmpty("tool_invocation_id", rec.SourceRef.ToolInvocationID)
	putNonEmpty("twin_approval_id", rec.SourceRef.TwinApprovalID)
	putNonEmpty("flow_trace_id", rec.SourceRef.FlowTraceID)
	putNonEmpty("suggestion_id", rec.SourceRef.SuggestionID)
	putNonEmpty("fact_id", rec.SourceRef.FactID)
	putNonEmpty("task_id", rec.SourceRef.TaskID)
	srcPB, err := structpb.NewStruct(src)
	if err != nil {
		return nil, err
	}
	msg.SourceRef = srcPB
	if len(rec.Metadata) > 0 {
		metaPB, err := structpb.NewStruct(rec.Metadata)
		if err != nil {
			return nil, err
		}
		msg.Metadata = metaPB
	}
	if t, ok := parseDecisionTimestamp(rec.CreatedAt); ok {
		msg.CreatedAt = timestamppb.New(t)
	}
	if t, ok := parseDecisionTimestamp(rec.UpdatedAt); ok {
		msg.UpdatedAt = timestamppb.New(t)
	}
	return msg, nil
}

// parseDecisionTimestamp 解析 TEXT 时间戳列（RFC3339/Nano）；失败返回 false
// （查询面容错：脏时间戳不回填，不落错误）。
func parseDecisionTimestamp(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
