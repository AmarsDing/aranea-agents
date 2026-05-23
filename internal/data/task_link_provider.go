package data

import "aranea-agents/internal/biz"

// ProvideTaskLinkRepo exposes task dependency links from the task repository.
func ProvideTaskLinkRepo(r biz.TaskRepo) biz.TaskLinkRepo {
	if l, ok := r.(biz.TaskLinkRepo); ok {
		return l
	}
	return nil
}
