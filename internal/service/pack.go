package service

import (
	"bytes"
	"context"

	packv1 "aranea-agents/api/kratos/pack/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// PackService implements packv1.PackServiceServer.
type PackService struct {
	packv1.UnimplementedPackServiceServer

	exporter      *pack.Exporter
	importer      *pack.Importer
	validatorRepo pack.ValidatorRepo
	lg            loggateway.Logger
	monitorBus    contract.MonitorBus
	gate          *evaluation.PublishGate
}

// NewPackService creates a new PackService.
// adapter must satisfy pack.ExporterRepo, pack.ImporterRepo, and pack.ValidatorRepo.
// lg / monitorBus feed the ecosystem.pack.install flow log; both are nil-safe
// (nil disables the process-log line / bus publication respectively).
// gate is the P2-1 publish regression gate; nil disables the check.
func NewPackService(adapter PackExporterImporterValidator, lg loggateway.Logger, monitorBus contract.MonitorBus, gate *evaluation.PublishGate) *PackService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &PackService{
		exporter:      pack.NewExporter(adapter),
		importer:      pack.NewImporter(adapter),
		validatorRepo: adapter,
		lg:            lg,
		monitorBus:    monitorBus,
		gate:          gate,
	}
}

// packInstallFlow returns a system-domain flow emitter for pack install
// events. Returns nil only when s is nil; a nil monitorBus still yields a
// valid emitter (bus publication is skipped downstream).
func (s *PackService) packInstallFlow(ctx context.Context) *event.TraceEmitter {
	if s == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
}

// PackExporterImporterValidator is the composite interface required by PackService.
type PackExporterImporterValidator interface {
	pack.ExporterRepo
	pack.ImporterRepo
	pack.ValidatorRepo
}

// ExportPack exports entities as a .arpack tar.gz.
func (s *PackService) ExportPack(ctx context.Context, req *packv1.ExportPackRequest) (*packv1.ExportPackResponse, error) {
	kind := req.GetKind()
	ref := req.GetRef()
	if kind == "" || ref == "" {
		return nil, apierror.BadRequest("PACK", "kind and ref are required")
	}

	var p *pack.Pack
	var err error

	switch kind {
	case "agent":
		p, err = s.exporter.ExportAgent(ctx, ref)
	case "team":
		p, err = s.exporter.ExportTeam(ctx, ref)
	case "industry":
		p, err = s.exporter.ExportIndustry(ctx, ref)
	default:
		return nil, apierror.BadRequest("PACK", "unsupported kind: "+kind)
	}
	if err != nil {
		return nil, apierror.Internal("PACK", "export failed: "+err.Error())
	}

	// Serialize to tar.gz
	var buf bytes.Buffer
	if err := pack.WritePack(p, &buf); err != nil {
		return nil, apierror.Internal("PACK", "serialize failed: "+err.Error())
	}

	return &packv1.ExportPackResponse{
		Data: buf.Bytes(),
		Name: p.Manifest.Name,
		Kind: p.Manifest.Kind,
	}, nil
}

// ImportPack imports a .arpack tar.gz.
func (s *PackService) ImportPack(ctx context.Context, req *packv1.ImportPackRequest) (*packv1.ImportPackResponse, error) {
	data := req.GetData()
	if len(data) == 0 {
		return nil, apierror.BadRequest("PACK", "data is required")
	}
	// 限制上传 Pack 大小
	if len(data) > pack.MaxPackSize {
		return nil, apierror.BadRequest("PACK", "pack file exceeds size limit")
	}

	flow := s.packInstallFlow(ctx)

	// Parse pack
	p, err := pack.ReadPack(bytes.NewReader(data))
	if err != nil {
		if flow != nil {
			flow.LogError("ecosystem.pack.install", "生态包安装失败：pack 解析错误",
				event.P("error", err.Error()),
			)
		}
		return nil, apierror.BadRequest("PACK", "invalid pack file: "+err.Error())
	}

	if flow != nil {
		flow.LogStart("ecosystem.pack.install", "开始安装生态包",
			event.P("pack", p.Manifest.Name),
			event.P("version", p.Manifest.Version),
		)
	}

	// Determine conflict strategy
	strategy := pack.ConflictSkip
	switch req.GetConflictStrategy() {
	case "overwrite":
		strategy = pack.ConflictOverwrite
	case "duplicate":
		strategy = pack.ConflictDuplicate
	}

	// P2-1: publish regression gate — blocked installs return Conflict and
	// nothing is imported. Nil gate / disabled config = no-op.
	if err := s.gate.Check(ctx, biz.EvalGateTriggerPackInstall); err != nil {
		if flow != nil {
			flow.LogError("ecosystem.pack.install", "生态包安装被评估门禁拦截",
				event.P("pack", p.Manifest.Name),
				event.P("version", p.Manifest.Version),
				event.P("error", err.Error()),
			)
		}
		return nil, err
	}

	// Import
	result, err := s.importer.Import(ctx, p, strategy)
	if err != nil {
		if flow != nil {
			flow.LogError("ecosystem.pack.install", "生态包安装失败",
				event.P("pack", p.Manifest.Name),
				event.P("version", p.Manifest.Version),
				event.P("error", err.Error()),
			)
		}
		return nil, apierror.Internal("PACK", "import failed: "+err.Error())
	}
	if flow != nil {
		flow.LogDone("ecosystem.pack.install", "生态包安装完成",
			event.P("pack", p.Manifest.Name),
			event.P("version", p.Manifest.Version),
			event.P("failures", len(result.Failures)),
		)
	}

	resp := &packv1.ImportPackResponse{
		OrgNodes:         int32(result.OrgNodes),
		AgentsCreated:    int32(result.AgentsCreated),
		AgentsUpdated:    int32(result.AgentsUpdated),
		AgentsSkipped:    int32(result.AgentsSkipped),
		GraphsCreated:    int32(result.GraphsCreated),
		TeamsCreated:     int32(result.TeamsCreated),
		TeamsUpdated:     int32(result.TeamsUpdated),
		TeamsSkipped:     int32(result.TeamsSkipped),
		ConflictStrategy: string(strategy),
	}
	for _, f := range result.Failures {
		resp.Failures = append(resp.Failures, &packv1.ImportFailure{
			EntityType: f.EntityType,
			Key:        f.Key,
			Reason:     f.Reason,
		})
	}
	return resp, nil
}

// ValidatePack dry-run validates a .arpack without importing.
func (s *PackService) ValidatePack(ctx context.Context, req *packv1.ValidatePackRequest) (*packv1.ValidatePackResponse, error) {
	data := req.GetData()
	if len(data) == 0 {
		return nil, apierror.BadRequest("PACK", "data is required")
	}
	// 限制上传 Pack 大小
	if len(data) > pack.MaxPackSize {
		return nil, apierror.BadRequest("PACK", "pack file exceeds size limit")
	}

	// Parse pack
	p, err := pack.ReadPack(bytes.NewReader(data))
	if err != nil {
		return nil, apierror.BadRequest("PACK", "invalid pack file: "+err.Error())
	}

	// Validate
	result, err := pack.Validate(ctx, p, s.validatorRepo)
	if err != nil {
		return nil, apierror.Internal("PACK", "validation failed: "+err.Error())
	}

	resp := &packv1.ValidatePackResponse{
		Valid:           result.Valid,
		Errors:          result.Errors,
		MissingSkills:   result.MissingSkills,
		MissingFuncRefs: result.MissingFuncRefs,
	}
	for _, c := range result.Conflicts {
		resp.Conflicts = append(resp.Conflicts, &packv1.ConflictItem{
			EntityType: c.EntityType,
			Key:        c.Key,
		})
	}
	return resp, nil
}
