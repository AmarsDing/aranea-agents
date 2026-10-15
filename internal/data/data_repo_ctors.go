package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/artifactfs"
)

func NewArtifactRepo(d *Data) biz.ArtifactRepo {
	return artifactfs.NewFSArtifactRepo(d.lg)
}

func NewKnowledgeRepoFromData(d *Data) biz.KnowledgeRepo {
	return NewKnowledgeRepo(d, d.lg)
}

func NewKnowledgeSparseSearcherFromData(d *Data) biz.KnowledgeSparseSearcher {
	repo := NewKnowledgeRepo(d, d.lg)
	if repo == nil {
		return nil
	}
	kr, ok := repo.(*knowledgeRepo)
	if !ok {
		return nil
	}
	return kr
}
