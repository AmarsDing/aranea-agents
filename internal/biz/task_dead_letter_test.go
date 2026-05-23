package biz

import "testing"

func TestShouldRecordTaskDeadLetter(t *testing.T) {
	if ShouldRecordTaskDeadLetter(`{"failure_policy":{"on_error":"await_review"}}`) {
		t.Fatal("await_review should not dead-letter")
	}
	if !ShouldRecordTaskDeadLetter(`{"failure_policy":{"on_error":"halt"}}`) {
		t.Fatal("halt should dead-letter")
	}
}
