package service

import "aranea-agents/internal/channel/lark"

func feishuAppAndRegion(configJSON string) (region, appID string, err error) {
	return lark.AppAndRegionFromConfig(configJSON)
}
