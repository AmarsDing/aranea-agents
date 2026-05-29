//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package hedge

import (
	"context"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

const defaultDelay = 100 * time.Millisecond

type options struct {
	candidates    []model.Model
	name          string
	contextWindow int
	delay         time.Duration
	delays        []time.Duration
	onSwitch      SwitchCallback
}

type SwitchCallback func(ctx context.Context, fromCandidate string, toCandidate string, err error)

func newOptions(opt ...Option) options {
	opts := options{
		delay: defaultDelay,
	}
	for _, o := range opt {
		o(&opts)
	}
	return opts
}

type Option func(*options)

func WithCandidates(candidates ...model.Model) Option {
	return func(o *options) {
		o.candidates = append(o.candidates, candidates...)
	}
}

func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

func WithContextWindow(tokens int) Option {
	return func(o *options) {
		if tokens > 0 {
			o.contextWindow = tokens
		}
	}
}

func WithDelay(delay time.Duration) Option {
	return func(o *options) {
		o.delay = delay
	}
}

func WithDelays(delays ...time.Duration) Option {
	return func(o *options) {
		o.delays = make([]time.Duration, len(delays))
		copy(o.delays, delays)
	}
}

func WithSwitchCallback(cb SwitchCallback) Option {
	return func(o *options) {
		o.onSwitch = cb
	}
}
