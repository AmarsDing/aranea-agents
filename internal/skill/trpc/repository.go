package trpc

import (
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

type FSRepositoryAdapter struct {
	root     string
	delegate *trpcskill.FSRepository
}

func NewFSRepositoryAdapter(root string) (*FSRepositoryAdapter, error) {
	repo, err := trpcskill.NewFSRepository(root)
	if err != nil {
		return nil, err
	}
	return &FSRepositoryAdapter{root: root, delegate: repo}, nil
}

func (a *FSRepositoryAdapter) Summaries() []trpcskill.Summary {
	return a.delegate.Summaries()
}

func (a *FSRepositoryAdapter) Get(name string) (*trpcskill.Skill, error) {
	return a.delegate.Get(name)
}

func (a *FSRepositoryAdapter) Path(name string) (string, error) {
	return a.delegate.Path(name)
}

func (a *FSRepositoryAdapter) Roots() []string {
	return a.delegate.Roots()
}

func (a *FSRepositoryAdapter) Refresh() error {
	return a.delegate.Refresh()
}
