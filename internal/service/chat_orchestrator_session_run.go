package service

import (
	"aranea-agents/internal/biz"
)

type sessionRunTurnBinding struct {
	sessionRunID string
	turnID       string
	agentID      string
	userContent  string
	dialogMode   string
	provider     string
	model        string
	runtimeRunID string
	ltCfg        biz.ChannelLongTaskConfig
}
