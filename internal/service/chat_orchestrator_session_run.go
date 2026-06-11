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

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
