package agenticons

import (
	"embed"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

//go:embed *.png
var fs embed.FS

func LoadPNG(assetKey string) ([]byte, error) {
	if assetKey == "" {
		return nil, kerrors.BadRequest("AGENT_ICONS", "asset key is required")
	}
	return fs.ReadFile(assetKey + ".png")
}
