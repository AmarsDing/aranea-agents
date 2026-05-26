package service

import (
	"context"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/agent/codeexecutor"
)

// CodeExecutorService handles code executor RPC delegation from MonitorService (SRP split).
type CodeExecutorService struct {
	factory *codeexecutor.Factory
}

// NewCodeExecutorService creates a CodeExecutorService backed by a codeexecutor.Factory.
func NewCodeExecutorService(factory *codeexecutor.Factory) *CodeExecutorService {
	return &CodeExecutorService{factory: factory}
}

func (s *CodeExecutorService) GetCodeExecutorCapabilities(_ context.Context, _ *v1.GetMonitorLogsRequest) (*v1.GetCodeExecutorCapabilitiesResponse, error) {
	factory := s.factory
	if factory == nil {
		factory = codeexecutor.NewFactory()
	}
	caps := factory.Capabilities()
	out := make([]*v1.CodeExecutorCapability, 0, len(caps))
	for _, c := range caps {
		out = append(out, &v1.CodeExecutorCapability{
			Type:      c.Type,
			Available: c.Available,
			Reason:    c.Reason,
		})
	}
	return &v1.GetCodeExecutorCapabilitiesResponse{Backends: out}, nil
}
