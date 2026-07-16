package biz

import (
	"aranea-agents/internal/biz/hook"
)

// Re-export hook types from sub-package for backward compatibility.
type (
	Hook                   = hook.Hook
	HookPatch              = hook.HookPatch
	HookRepo               = hook.Repo
	HookUsecase            = hook.Usecase
	HookListQuery          = hook.ListQuery
	HookListResult         = hook.ListResult
	HookConfig             = hook.Config
	HookCondition          = hook.Condition
	HookAction             = hook.Action
	HookDelivery           = hook.Delivery
	HookDeliveryStatus     = hook.DeliveryStatus
	HookDeliveryQuery      = hook.DeliveryQuery
	HookDeliveryListResult = hook.DeliveryListResult
	HookDeliveryRepo       = hook.DeliveryRepo
	HookNotifyOptions      = hook.NotifyOptions
	HookDeliveryUsecase    = hook.DeliveryUsecase
	ResolvedHook           = hook.ResolvedHook
	HookResolver           = hook.Resolver
)

// Re-export hook constants for backward compatibility.
const (
	HookDeliveryPending = hook.DeliveryPending
	HookDeliverySuccess = hook.DeliverySuccess
	HookDeliveryFailed  = hook.DeliveryFailed
)

// Re-export hook constructors and helpers for backward compatibility.
var (
	NewHookUsecase              = hook.NewUsecase
	NewHookDeliveryUsecase      = hook.NewDeliveryUsecase
	NewHookResolver             = hook.NewResolver
	ParseHookConfig             = hook.ParseConfig
	ValidateHookConfigForSave   = hook.ValidateConfigForSave
	NormalizeCallbackPoint      = hook.NormalizeCallbackPoint
	HookAppliesToAgent          = hook.AppliesToAgent
	HookAppliesToTool           = hook.AppliesToTool
	ParseHookNotifyOptions      = hook.ParseNotifyOptions
	NormalizeHookDeliveryStatus = hook.NormalizeDeliveryStatus
)
