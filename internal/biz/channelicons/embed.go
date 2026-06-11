package channelicons

import (
	"embed"

	"aranea-agents/pkg/apierror"
)

//go:embed *.png
var fs embed.FS

// LoadPNG returns embedded channel platform icon bytes by asset_key (e.g. channel_feishu).
func LoadPNG(assetKey string) ([]byte, error) {
	if assetKey == "" {
		return nil, apierror.BadRequest("CHANNEL_ICONS", "asset key is required")
	}
	return fs.ReadFile(assetKey + ".png")
}
