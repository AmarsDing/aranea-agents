package biz

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestProcessAvatarUploadSquareResize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			src.Set(x, y, image.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	main, thumb, w, h, mime, err := processAvatarUpload(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime=%q", mime)
	}
	if w != 40 || h != 40 {
		t.Fatalf("main size=%dx%d want 40x40 (no upscale)", w, h)
	}
	if len(main) == 0 || len(thumb) == 0 {
		t.Fatal("expected encoded payloads")
	}
}
