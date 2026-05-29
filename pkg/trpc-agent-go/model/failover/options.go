//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package failover

import (
	"context"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

type options struct {
	candidates    []model.Model
	onSwitch      SwitchCallback
}

type SwitchCallback func(ctx context.Context, fromCandidate string, toCandidate string, err error)

type Option func(*options)

func WithCandidates(candidates ...model.Model) Option {
	return func(o *options) {
		o.candidates = append(o.candidates, candidates...)
	}
}

func WithSwitchCallback(cb SwitchCallback) Option {
	return func(o *options) {
		o.onSwitch = cb
	}
}
