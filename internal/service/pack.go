package service

import (
	"bytes"
	"context"

	packv1 "aranea-agents/api/kratos/pack/v1"
	"aranea-agents/internal/biz/pack"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// PackService implements packv1.PackServiceServer.
type PackService struct {
	packv1.UnimplementedPackServiceServer

	exporter *pack.Exporter
	importer *pack.Importer
	validatorRepo pack.ValidatorRepo
}

// NewPackService creates a new PackService.
// adapter must satisfy pack.ExporterRepo, pack.ImporterRepo, and pack.ValidatorRepo.
func NewPackService(adapter packExporterImporterValidator) *PackService {
	return &PackService{
		exporter:      pack.NewExporter(adapter),
		importer:      pack.NewImporter(adapter),
		validatorRepo: adapter,
	}
}

// packExporterImporterValidator is the composite interface required by PackService.
type packExporterImporterValidator interface {
	pack.ExporterRepo
	pack.ImporterRepo
	pack.ValidatorRepo
}

// ExportPack exports entities as a .arpack tar.gz.
func (s *PackService) ExportPack(ctx context.Context, req *packv1.ExportPackRequest) (*packv1.ExportPackResponse, error) {
	kind := req.GetKind()
	ref := req.GetRef()
	if kind == "" || ref == "" {
		return nil, kerrors.BadRequest("PACK", "kind and ref are required")
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
		return nil, kerrors.BadRequest("PACK", "unsupported kind: "+kind)
	}
	if err != nil {
		return nil, kerrors.InternalServer("PACK", "export failed: "+err.Error())
	}

	// Serialize to tar.gz
	var buf bytes.Buffer
	if err := pack.WritePack(p, &buf); err != nil {
		return nil, kerrors.InternalServer("PACK", "serialize failed: "+err.Error())
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
		return nil, kerrors.BadRequest("PACK", "data is required")
	}
	// 限制上传 Pack 大小（200MB）
	if len(data) > 200*1024*1024 {
		return nil, kerrors.BadRequest("PACK", "pack file exceeds 200MB limit")
	}

	// Parse pack
	p, err := pack.ReadPack(bytes.NewReader(data))
	if err != nil {
		return nil, kerrors.BadRequest("PACK", "invalid pack file: "+err.Error())
	}

	// Determine conflict strategy
	strategy := pack.ConflictSkip
	switch req.GetConflictStrategy() {
	case "overwrite":
		strategy = pack.ConflictOverwrite
	case "duplicate":
		strategy = pack.ConflictDuplicate
	}

	// Import
	result, err := s.importer.Import(ctx, p, strategy)
	if err != nil {
		return nil, kerrors.InternalServer("PACK", "import failed: "+err.Error())
	}

	resp := &packv1.ImportPackResponse{
		TaxonomyNodes:   int32(result.TaxonomyNodes),
		AgentsCreated:   int32(result.AgentsCreated),
		AgentsUpdated:   int32(result.AgentsUpdated),
		AgentsSkipped:   int32(result.AgentsSkipped),
		GraphsCreated:   int32(result.GraphsCreated),
		TeamsCreated:    int32(result.TeamsCreated),
		TeamsUpdated:    int32(result.TeamsUpdated),
		TeamsSkipped:    int32(result.TeamsSkipped),
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
		return nil, kerrors.BadRequest("PACK", "data is required")
	}
	// 限制上传 Pack 大小（200MB）
	if len(data) > 200*1024*1024 {
		return nil, kerrors.BadRequest("PACK", "pack file exceeds 200MB limit")
	}

	// Parse pack
	p, err := pack.ReadPack(bytes.NewReader(data))
	if err != nil {
		return nil, kerrors.BadRequest("PACK", "invalid pack file: "+err.Error())
	}

	// Validate
	result, err := pack.Validate(ctx, p, s.validatorRepo)
	if err != nil {
		return nil, kerrors.InternalServer("PACK", "validation failed: "+err.Error())
	}

	resp := &packv1.ValidatePackResponse{
		Valid:          result.Valid,
		Errors:         result.Errors,
		MissingSkills:  result.MissingSkills,
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
