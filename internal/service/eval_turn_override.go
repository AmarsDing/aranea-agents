package service

import (
	"encoding/json"

	"aranea-agents/internal/biz"
)

func applyEvalOverrideToAgent(ag *biz.Agent, ov biz.EvalRunOverride) {
	if ag == nil || ov.Tools == "" {
		return
	}
	if ag.Settings == nil {
		return
	}
	cp := *ag.Settings
	ag.Settings = &cp
	none, allow := biz.ParseEvalToolsOverride(ov.Tools)
	if none {
		ag.Settings.ToolsEnabled = false
		return
	}
	if len(allow) == 0 {
		return
	}
	raw, err := json.Marshal(allow)
	if err != nil {
		return
	}
	ag.Settings.ToolsAllowJSON = string(raw)
	ag.Settings.ToolsEnabled = true
}
