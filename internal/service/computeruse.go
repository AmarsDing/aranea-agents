package service

// computeruse.go — Computer Use（75-computer-use）装配适配。
//
// Usecase 本体在 internal/biz/computeruse；sidecar 进程/RPC 在
// internal/computeruse。本文件把 service 层可用协作者（流程日志 writer）
// 适配进 Usecase Deps，并按环境变量定位 sidecar 可执行文件。
// sidecar 懒拉起：构造不产生子进程，非 Windows 平台仅在首次调用时
// 返回明确错误，不影响服务启动。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	computerusev1 "aranea-agents/api/kratos/computeruse/v1"
	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	cuinfra "aranea-agents/internal/computeruse"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
)

// defaultCUASidecarPath sidecar 默认产物路径（AGENTS.md bin/ 约定）。
const defaultCUASidecarPath = "bin/cua/aranea-cua-win.exe"

// defaultOmniParserURL OmniParser V2 omniparserserver 默认地址（独立部署）。
// 端口 8101：8100 被本机 twin aiops.exe 常驻占用。
const defaultOmniParserURL = "http://127.0.0.1:8101"

// ProvideComputerUseUsecase 构造进程级 Computer Use 用例编排器。
// flow 可 nil（跳过流程日志）；audit 可 nil（跳过审计落库，M1.4 已接线 Ent repo）；
// monitorBus 可 nil（跳过 computeruse.step 实时事件，M1.4 已接线 MonitorBus）。
// Vision（OmniParser）不可用时按 Available 探测自动降级；Grounder（VLM）
// 经 LLM catalog 解析视觉模型，sys/catalog 与 knowledge 模块同签名接口复用。
func ProvideComputerUseUsecase(flow biz.FlowLogWriter, llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, audit bizcu.AuditStore, monitorBus contract.MonitorBus, lg loggateway.Logger) *bizcu.ComputerUseUsecase {
	path := os.Getenv("ARANEA_CUA_SIDECAR_PATH")
	if path == "" {
		path = defaultCUASidecarPath
	}
	omniURL := os.Getenv("ARANEA_CUA_OMNIPARSER_URL")
	if omniURL == "" {
		omniURL = defaultOmniParserURL
	}
	mgr := cuinfra.NewManager(path, lg)
	return bizcu.NewComputerUseUsecase(bizcu.Deps{
		Gateway:  cuinfra.NewGateway(mgr, lg),
		Match:    cuinfra.NewMatcher(),
		Vision:   cuinfra.NewOmniParserClient(omniURL, lg),
		Grounder: cuinfra.NewVLMGrounder(llm, sys, catalog, lg),
		Audit:    audit,
		Events:   cuinfra.NewStepEventPublisher(monitorBus),
		FlowLog:  flow,
		Lg:       lg,
	})
}

// ---------------------------------------------------------------------------
// HTTP/gRPC 服务（设计 §3.9）：kill / steps / status。
// ---------------------------------------------------------------------------

// ComputerUseService 实现 computerusev1.ComputerUseServiceServer。
type ComputerUseService struct {
	computerusev1.UnimplementedComputerUseServiceServer

	uc *bizcu.ComputerUseUsecase
}

// NewComputerUseService 构造；uc 由 ProvideComputerUseUsecase 提供。
func NewComputerUseService(uc *bizcu.ComputerUseUsecase) *ComputerUseService {
	return &ComputerUseService{uc: uc}
}

// KillComputerUseSession 急停会话。
func (s *ComputerUseService) KillComputerUseSession(ctx context.Context, req *computerusev1.KillComputerUseSessionRequest) (*computerusev1.KillComputerUseSessionResponse, error) {
	if req.GetId() == "" {
		return nil, apierror.BadRequest(apierror.DomainComputerUse, "id 必填")
	}
	if err := s.uc.KillSwitch(ctx, req.GetId()); err != nil {
		return nil, computerUseErr(err)
	}
	sess, err := s.uc.GetSession(ctx, req.GetId())
	if err != nil {
		return nil, computerUseErr(err)
	}
	return &computerusev1.KillComputerUseSessionResponse{SessionId: sess.ID, Status: string(sess.Status)}, nil
}

// ListComputerUseSteps 审计步骤查询。
func (s *ComputerUseService) ListComputerUseSteps(ctx context.Context, req *computerusev1.ListComputerUseStepsRequest) (*computerusev1.ListComputerUseStepsResponse, error) {
	if req.GetId() == "" {
		return nil, apierror.BadRequest(apierror.DomainComputerUse, "id 必填")
	}
	steps, err := s.uc.ListSteps(ctx, req.GetId())
	if err != nil {
		return nil, computerUseErr(err)
	}
	resp := &computerusev1.ListComputerUseStepsResponse{Items: make([]*computerusev1.ComputerUseStep, 0, len(steps))}
	for _, st := range steps {
		resp.Items = append(resp.Items, stepToProto(st))
	}
	return resp, nil
}

// GetComputerUseStatus sidecar / 视觉组件健康状态。
func (s *ComputerUseService) GetComputerUseStatus(ctx context.Context, _ *emptypb.Empty) (*computerusev1.GetComputerUseStatusResponse, error) {
	st := s.uc.Status(ctx)
	resp := &computerusev1.GetComputerUseStatusResponse{}
	if v, ok := st["sidecar"].(string); ok {
		resp.Sidecar = v
	}
	if v, ok := st["platform"].(string); ok {
		resp.Platform = v
	}
	if v, ok := st["scale_factor"].(float64); ok {
		resp.ScaleFactor = v
	}
	if v, ok := st["vision"].(bool); ok {
		resp.Vision = v
	}
	return resp, nil
}

func stepToProto(st bizcu.AuditEntry) *computerusev1.ComputerUseStep {
	var paramsJSON string
	if len(st.Params) > 0 {
		if b, err := json.Marshal(st.Params); err == nil {
			paramsJSON = string(b)
		}
	}
	return &computerusev1.ComputerUseStep{
		Id:          st.ID,
		SessionId:   st.SessionID,
		AgentKey:    st.AgentKey,
		StepIndex:   int32(st.Index),
		Target:      st.Target,
		Path:        string(st.Path),
		Action:      string(st.Action),
		ParamsJson:  paramsJSON,
		Result:      string(st.Result),
		Error:       st.Error,
		DurationMs:  st.DurationMs,
		ConfirmedBy: st.ConfirmedBy,
		Danger:      st.Danger,
		CreatedAt:   st.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// computerUseErr biz 错误 → apierror（Kratos 经中间件转 HTTP 状态码）。
func computerUseErr(err error) error {
	switch {
	case errors.Is(err, bizcu.ErrSessionNotFound):
		return apierror.NotFound(apierror.DomainComputerUse, "%s", err.Error())
	case errors.Is(err, bizcu.ErrBudgetExceeded),
		errors.Is(err, bizcu.ErrSessionCancelled),
		errors.Is(err, bizcu.ErrSessionTerminal),
		errors.Is(err, bizcu.ErrBlockedProcess):
		return apierror.FailedPrecondition(apierror.DomainComputerUse, "%s", err.Error())
	default:
		return err
	}
}
