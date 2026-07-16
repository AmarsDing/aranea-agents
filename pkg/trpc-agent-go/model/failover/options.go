//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package failover

import "trpc.group/trpc-go/trpc-agent-go/model"

// SwitchCallback is invoked when failover switches from a failed candidate to
// the next candidate. fromIndex/toIndex are positions in the candidate list;
// reason is a short machine-readable cause (e.g. response error message).
type SwitchCallback func(fromIndex, toIndex int, fromName, toName, reason string)

type options struct {
	candidates []model.Model
	onSwitch   SwitchCallback
}

// Option configures a failover model.
type Option func(*options)

// WithCandidates appends failover candidates in priority order.
func WithCandidates(candidates ...model.Model) Option {
	return func(o *options) {
		o.candidates = append(o.candidates, candidates...)
	}
}

// WithSwitchCallback registers a callback fired on each failover switch.
func WithSwitchCallback(cb SwitchCallback) Option {
	return func(o *options) {
		o.onSwitch = cb
	}
}
