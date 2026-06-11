package biz

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"time"

	"aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/biz/channelicons"
	"aranea-agents/pkg/apierror"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const (
	iconifyAPIBase  = "https://api.iconify.design/simple-icons"
	iconCanvasSize  = 256
	iconPadding     = 48
	iconHTTPTimeout = 30 * time.Second
)

// simpleIconsSlug maps channel type to Simple Icons slug (https://simpleicons.org).
var simpleIconsSlug = map[string]string{
	"qq":              "qq",
	"qqbot":           "qq",
	"feishu":          "bytedance",
	"dingtalk":        "alibabadotcom",
	"wecom":           "wechat",
	"wecom-app":       "wechat",
	"openclaw-weixin": "wechat",
	"wechat":          "wechat",
	"telegram":        "telegram",
	"whatsapp":        "whatsapp",
	"facebook":        "messenger",
	"discord":         "discord",
	"slack":           "slack",
	"msteams":         "microsoftteams",
	"googlechat":      "googlechat",
	"line":            "line",
	"matrix":          "matrix",
	"mattermost":      "mattermost",
	"signal":          "signal",
	"zalo":            "zalo",
	"zalouser":        "zalo",
	"imessage":        "imessage",
	"bluebubbles":     "imessage",
	"nextcloud-talk":  "nextcloud",
	"synology-chat":   "synology",
	"irc":             "liberadotchat",
	"nostr":           "nostr",
	"twitch":          "twitch",
	"tlon":            "urbit",
	"voice-call":      "twilio",
	"qa-channel":      "testinglibrary",
}

// channelIconRefresher implements avatar.ChannelIconRefresher via Iconify API.
type channelIconRefresher struct{}

// NewChannelIconRefresher constructs a ChannelIconRefresher implementation.
func NewChannelIconRefresher() avatar.ChannelIconRefresher {
	return &channelIconRefresher{}
}

// RefreshChannelPlatformIcons fetches channel icons from Iconify and upserts them.
func (r *channelIconRefresher) RefreshChannelPlatformIcons(ctx context.Context, repo avatar.Repo) (*avatar.RefreshChannelPlatformIconsResult, error) {
	if repo == nil {
		return nil, apierror.BadRequest("AVATAR", "avatar repo is required")
	}
	client := &http.Client{Timeout: iconHTTPTimeout}
	result := &avatar.RefreshChannelPlatformIconsResult{}

	for _, spec := range ChannelPlatformAvatarSpecs() {
		pngData, err := fetchAndRenderChannelIcon(ctx, client, spec)
		if err != nil {
			result.Failed++
			continue
		}
		if err := upsertChannelIconFromPNG(ctx, repo, spec, pngData); err != nil {
			result.Failed++
			continue
		}
		result.Updated++
	}
	return result, nil
}

// fetchAndRenderChannelIcon downloads an SVG from Iconify and renders it to PNG.
// Falls back to the embedded PNG or the text-label renderer on failure.
func fetchAndRenderChannelIcon(ctx context.Context, client *http.Client, spec ChannelPlatformAvatarSpec) ([]byte, error) {
	slug := simpleIconsSlug[spec.ChannelType]
	if slug == "" {
		return loadEmbeddedOrFallbackIcon(spec)
	}

	svg, err := downloadIconifySVG(ctx, client, slug)
	if err != nil {
		return loadEmbeddedOrFallbackIcon(spec)
	}

	pngData, err := renderBrandIcon(svg, spec.R, spec.G, spec.B)
	if err != nil {
		return loadEmbeddedOrFallbackIcon(spec)
	}
	return pngData, nil
}

func loadEmbeddedOrFallbackIcon(spec ChannelPlatformAvatarSpec) ([]byte, error) {
	if data, err := channelicons.LoadPNG(spec.AssetKey); err == nil {
		return data, nil
	}
	return RenderChannelPlatformAvatarPNG(spec)
}

func downloadIconifySVG(ctx context.Context, client *http.Client, slug string) ([]byte, error) {
	u := iconifyAPIBase + "/" + slug + ".svg"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apierror.Unavailable("AVATAR", "iconify API returned %d for %s", resp.StatusCode, u)
	}
	return io.ReadAll(resp.Body)
}

func renderBrandIcon(svgData []byte, r, g, b uint8) ([]byte, error) {
	svgData = bytes.ReplaceAll(svgData, []byte(`fill="currentColor"`), []byte(`fill="#FFFFFF"`))
	svgData = bytes.ReplaceAll(svgData, []byte(`width="1em"`), []byte(`width="24"`))
	svgData = bytes.ReplaceAll(svgData, []byte(`height="1em"`), []byte(`height="24"`))

	icon, err := oksvg.ReadIconStream(bytes.NewReader(svgData))
	if err != nil {
		return nil, err
	}

	size := float64(iconCanvasSize - iconPadding*2)
	icon.SetTarget(0, 0, size, size)

	img := image.NewRGBA(image.Rect(0, 0, iconCanvasSize, iconCanvasSize))
	bg := color.RGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < iconCanvasSize; y++ {
		for x := 0; x < iconCanvasSize; x++ {
			img.Set(x, y, bg)
		}
	}

	sub := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	scanner := rasterx.NewScannerGV(int(size), int(size), sub, sub.Bounds())
	raster := rasterx.NewDasher(int(size), int(size), scanner)
	icon.Draw(raster, 1)

	offset := iconPadding
	for y := 0; y < int(size); y++ {
		for x := 0; x < int(size); x++ {
			c := sub.At(x, y)
			if _, _, _, a := c.RGBA(); a > 0 {
				img.Set(x+offset, y+offset, color.White)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// upsertChannelIconFromPNG processes the PNG data and upserts into avatar_assets.
func upsertChannelIconFromPNG(ctx context.Context, repo avatar.Repo, spec ChannelPlatformAvatarSpec, pngData []byte) error {
	main, thumb, w, h, mime, err := avatar.ProcessAvatarUpload(pngData, "image/png")
	if err != nil {
		return err
	}

	existing, err := repo.GetAvatarAssetByKey(ctx, spec.AssetKey)
	if err == nil && existing.ID != "" {
		return repo.UpdateAvatarAssetImages(ctx, existing.ID, main, thumb, mime, w, h, len(main))
	}
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return err
	}

	asset := avatar.Asset{
		ID:            spec.AssetKey,
		Key:           spec.AssetKey,
		Name:          spec.Name,
		Description:   "Channel platform icon (" + spec.ChannelType + ")",
		MimeType:      mime,
		Source:        "system",
		Category:      "channel",
		IsSystem:      true,
		FileSizeBytes: len(main),
		WidthPx:       w,
		HeightPx:      h,
		SortOrder:     spec.SortOrder,
	}
	_, err = repo.CreateAvatarAsset(ctx, asset, main, thumb)
	return err
}
