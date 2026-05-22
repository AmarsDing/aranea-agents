package biz

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const channelIconCanvasPx = 128

// RenderChannelPlatformAvatarPNG draws a rounded-square platform badge (128×128 PNG).
func RenderChannelPlatformAvatarPNG(spec ChannelPlatformAvatarSpec) ([]byte, error) {
	size := channelIconCanvasPx
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{R: spec.R, G: spec.G, B: spec.B, A: 255}
	fillRoundedRect(img, image.Rect(0, 0, size, size), size/5, bg)

	label := spec.Label
	if label == "" {
		label = "?"
	}
	face, err := channelIconFontFace(label)
	if err != nil {
		return nil, err
	}
	bounds, _ := font.BoundString(face, label)
	textW := (bounds.Max.X - bounds.Min.X).Ceil()
	textH := (bounds.Max.Y - bounds.Min.Y).Ceil()
	x := (size - textW) / 2
	y := (size+textH)/2 - bounds.Max.Y.Ceil()
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(label)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func channelIconFontFace(label string) (font.Face, error) {
	ft, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	size := 42.0
	if len([]rune(label)) > 1 {
		size = 32.0
	}
	return opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func fillRoundedRect(img *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	if radius < 1 {
		draw.Draw(img, rect, &image.Uniform{col}, image.Point{}, draw.Src)
		return
	}
	// Fill center cross + four corner circles for a simple rounded square.
	draw.Draw(img, image.Rect(rect.Min.X+radius, rect.Min.Y, rect.Max.X-radius, rect.Max.Y), &image.Uniform{col}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(rect.Min.X, rect.Min.Y+radius, rect.Max.X, rect.Max.Y-radius), &image.Uniform{col}, image.Point{}, draw.Src)
	for _, c := range []image.Point{
		{rect.Min.X + radius, rect.Min.Y + radius},
		{rect.Max.X - radius - 1, rect.Min.Y + radius},
		{rect.Min.X + radius, rect.Max.Y - radius - 1},
		{rect.Max.X - radius - 1, rect.Max.Y - radius - 1},
	} {
		fillCircle(img, c, radius, col)
	}
}

func fillCircle(img *image.RGBA, center image.Point, radius int, col color.Color) {
	r2 := float64(radius * radius)
	b := image.Rect(center.X-radius, center.Y-radius, center.X+radius+1, center.Y+radius+1)
	b = b.Intersect(img.Bounds())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x - center.X)
			dy := float64(y - center.Y)
			if dx*dx+dy*dy <= r2+0.5 {
				img.Set(x, y, col)
			}
		}
	}
}
