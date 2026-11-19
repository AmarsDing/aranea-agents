package review

import (
	"errors"
	"fmt"
)

// ErrIllegalTransition 状态机不允许的迁移哨兵（service 层映 409——与并发
// 决议同语义：请求基于过期状态视图）。
var ErrIllegalTransition = errors.New("review: illegal transition")

var transitions = map[string]map[string]bool{
	StatusPending:   {StatusApproved: true, StatusRejected: true, StatusCancelled: true},
	StatusApproved:  {},
	StatusRejected:  {},
	StatusCancelled: {},
}

func CanTransition(from, to string) bool {
	next, ok := transitions[from]
	return ok && next[to]
}

func Transition(from, to string) (string, error) {
	if !CanTransition(from, to) {
		return from, fmt.Errorf("%w: %s → %s", ErrIllegalTransition, from, to)
	}
	return to, nil
}
