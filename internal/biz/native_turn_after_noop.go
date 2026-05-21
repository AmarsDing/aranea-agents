package biz

import "context"

// NoopNativeTurnAfter is used until evaluation AfterTurn hook is attached (wire cycle break).
type NoopNativeTurnAfter struct{}

func (NoopNativeTurnAfter) AfterNativeTurn(context.Context, NativeTurnEvent) {}
