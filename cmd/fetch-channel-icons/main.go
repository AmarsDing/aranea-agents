// fetch-channel-icons downloads Simple Icons SVGs and writes PNGs for channel platform seed.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"aranea-agents/internal/biz"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
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

const (
	outDir      = "internal/biz/channelicons"
	iconifyAPI  = "https://api.iconify.design/simple-icons"
	canvasSize  = 256
	iconPadding = 48
)

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	ok, fail := 0, 0
	for _, spec := range biz.ChannelPlatformAvatarSpecs() {
		slug := simpleIconsSlug[spec.ChannelType]
		if slug == "" {
			fmt.Printf("skip %s: no slug mapping\n", spec.ChannelType)
			fail++
			continue
		}
		svg, err := downloadSVG(client, slug)
		if err != nil {
			fmt.Printf("warn %s (%s): download: %v — using fallback\n", spec.AssetKey, slug, err)
			if err := writeFallback(spec); err != nil {
				fatal(err)
			}
			ok++
			continue
		}
		pngData, err := renderBrandIcon(svg, spec.R, spec.G, spec.B)
		if err != nil {
			fmt.Printf("warn %s: render: %v — using fallback\n", spec.AssetKey, err)
			if err := writeFallback(spec); err != nil {
				fatal(err)
			}
			ok++
			continue
		}
		outPath := filepath.Join(outDir, spec.AssetKey+".png")
		if err := os.WriteFile(outPath, pngData, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("ok %s (%s) %d bytes\n", spec.AssetKey, slug, len(pngData))
		ok++
	}
	fmt.Printf("done: %d ok, %d unmapped\n", ok, fail)
}

func downloadSVG(client *http.Client, slug string) ([]byte, error) {
	u := iconifyAPI + "/" + slug + ".svg"
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d for %s", resp.StatusCode, u)
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
	size := float64(canvasSize - iconPadding*2)
	icon.SetTarget(0, 0, size, size)
	img := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	bg := color.RGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < canvasSize; y++ {
		for x := 0; x < canvasSize; x++ {
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

func writeFallback(spec biz.ChannelPlatformAvatarSpec) error {
	pngData, err := biz.RenderChannelPlatformAvatarPNG(spec)
	if err != nil {
		return err
	}
	outPath := filepath.Join(outDir, spec.AssetKey+".png")
	return os.WriteFile(outPath, pngData, 0o644)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
