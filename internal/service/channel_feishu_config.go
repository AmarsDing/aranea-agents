package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/channel/lark"
)

func feishuAppAndRegion(configJSON string) (region, appID string, err error) {
	var top struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &top); err != nil {
		return "", "", fmt.Errorf("feishu config: %w", err)
	}
	if top.Config == nil {
		return "", "", fmt.Errorf("feishu config_json.config is required (app_id)")
	}
	appID = strings.TrimSpace(fmt.Sprint(top.Config["app_id"]))
	if appID == "" {
		return "", "", fmt.Errorf("feishu config_json.config.app_id is required")
	}
	region = strings.TrimSpace(strings.ToLower(fmt.Sprint(top.Config["region"])))
	if region == "" {
		region = lark.RegionFeishu
	}
	return region, appID, nil
}
