package service

import "aranea-agents/internal/biz"

type compositeGraphExecutionObserver []biz.GraphExecutionObserver

func (o compositeGraphExecutionObserver) OnGraphExecutionComplete(exec *biz.GraphExecution) {
	for _, obs := range o {
		if obs == nil {
			continue
		}
		obs.OnGraphExecutionComplete(exec)
	}
}
