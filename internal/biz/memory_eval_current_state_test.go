package biz

import "testing"

func TestEvalQueryAsksCurrentState(t *testing.T) {
	t.Parallel()
	for _, q := range []string{
		"Which city do I live in now?",
		"What is my favorite color now?",
		"What is my current address?",
		"我现在住在哪里？",
		"目前偏好是什么",
	} {
		if !EvalQueryAsksCurrentState(q) {
			t.Errorf("want current-state: %q", q)
		}
	}
	for _, q := range []string{
		"Where did I previously live?",
		"What was my favorite color on 2024-02-15?",
		"How did my preference evolve?",
		"I used to live in Berlin",
	} {
		if EvalQueryAsksCurrentState(q) {
			t.Errorf("must not mark as current-state: %q", q)
		}
	}
}
