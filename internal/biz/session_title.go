package biz

import "context"

type SessionTitleGenerator interface {
	Generate(ctx context.Context, userMessage string) (string, error)
}

type noopSessionTitleGenerator struct{}

func NewNoopSessionTitleGenerator() SessionTitleGenerator {
	return &noopSessionTitleGenerator{}
}

func (noopSessionTitleGenerator) Generate(_ context.Context, _ string) (string, error) {
	return "", nil
}
