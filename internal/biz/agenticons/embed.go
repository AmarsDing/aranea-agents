package agenticons

import (
	"embed"

	"aranea-agents/pkg/apierror"
)

//go:embed *.png
var fs embed.FS

func LoadPNG(assetKey string) ([]byte, error) {
	if assetKey == "" {
		return nil, apierror.BadRequest("AGENT_ICONS", "asset key is required")
	}
	return fs.ReadFile(assetKey + ".png")
}
