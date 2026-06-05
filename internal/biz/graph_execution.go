package biz

func upsertGraphStep(steps []GraphStepSnapshot, step GraphStepSnapshot) []GraphStepSnapshot {
	for i := range steps {
		if steps[i].NodeID == step.NodeID && steps[i].StepIndex == step.StepIndex {
			steps[i] = step
			return steps
		}
	}
	return append(steps, step)
}
