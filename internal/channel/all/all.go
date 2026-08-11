// Package all registers every channel platform connector via init().
package all

import (
	_ "aranea-agents/internal/channel/dingtalk"
	_ "aranea-agents/internal/channel/discord"
	_ "aranea-agents/internal/channel/lark"
	_ "aranea-agents/internal/channel/line"
	_ "aranea-agents/internal/channel/mattermost"
	_ "aranea-agents/internal/channel/slack"
	_ "aranea-agents/internal/channel/teams"
	_ "aranea-agents/internal/channel/telegram"
	_ "aranea-agents/internal/channel/wechatilink"
)
