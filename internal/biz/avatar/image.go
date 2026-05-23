package avatar

import (
	"bytes"
	"image"
	_ "image/gif"
	"image/jpeg"

	"github.com/go-kratos/kratos/v2/errors"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	avatarMainMaxPx  = 512
	avatarThumbMaxPx = 128
)

// ProcessAvatarUpload decodes, center-crops square, and emits main + thumbnail payloads.
func ProcessAvatarUpload(data []byte, mime string) (main []byte, thumb []byte, width, height int, outMime string, err error) {
	if len(data) == 0 {
		return nil, nil, 0, 0, "", errors.BadRequest("AVATAR", "avatar file is required")
	}
	img, _, decErr := image.Decode(bytes.NewReader(data))
	if decErr != nil {
		return nil, nil, 0, 0, "", errors.BadRequest("AVATAR", "invalid avatar image")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, nil, 0, 0, "", errors.BadRequest("AVATAR", "invalid avatar dimensions")
	}
	side := w
	if h < side {
		side = h
	}
	sx := b.Min.X + (w-side)/2
	sy := b.Min.Y + (h-side)/2
	cropRect := image.Rect(0, 0, side, side)
	cropped := image.NewRGBA(cropRect)
	draw.CatmullRom.Scale(cropped, cropRect, img, image.Rect(sx, sy, sx+side, sy+side), draw.Over, nil)

	mainImg := resizeSquare(cropped, avatarMainMaxPx)
	thumbImg := resizeSquare(cropped, avatarThumbMaxPx)
	outMime = "image/jpeg"
	main, err = encodeJPEG(mainImg, 92)
	if err != nil {
		return nil, nil, 0, 0, "", err
	}
	thumb, err = encodeJPEG(thumbImg, 88)
	if err != nil {
		return nil, nil, 0, 0, "", err
	}
	return main, thumb, mainImg.Bounds().Dx(), mainImg.Bounds().Dy(), outMime, nil
}

func resizeSquare(src image.Image, maxEdge int) *image.RGBA {
	b := src.Bounds()
	side := b.Dx()
	if side <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	outEdge := side
	if outEdge > maxEdge {
		outEdge = maxEdge
	}
	dst := image.NewRGBA(image.Rect(0, 0, outEdge, outEdge))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, errors.InternalServer("AVATAR", "encode avatar jpeg failed")
	}
	return buf.Bytes(), nil
}
