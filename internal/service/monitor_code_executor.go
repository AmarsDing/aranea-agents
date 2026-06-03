package service

import (
	"context"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/pkg/loggateway"
)

// CodeExecutorService handles code executor RPC delegation from MonitorService (SRP split).
type CodeExecutorService struct {
	factory *codeexecutor.Factory
	lg      loggateway.Logger
}

// NewCodeExecutorService creates a CodeExecutorService backed by a codeexecutor.Factory.
func NewCodeExecutorService(factory *codeexecutor.Factory, lg loggateway.Logger) *CodeExecutorService {
	return &CodeExecutorService{factory: factory, lg: lg}
}

func (s *CodeExecutorService) GetCodeExecutorCapabilities(_ context.Context, _ *v1.GetMonitorLogsRequest) (*v1.GetCodeExecutorCapabilitiesResponse, error) {
	factory := s.factory
	if factory == nil {
		factory = codeexecutor.NewFactoryWithLogger(s.lg)
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
