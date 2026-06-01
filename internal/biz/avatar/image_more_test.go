package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestProcessAvatarUpload_emptyData(t *testing.T) {
	_, _, _, _, _, err := ProcessAvatarUpload(nil, "image/png")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestProcessAvatarUpload_emptySlice(t *testing.T) {
	_, _, _, _, _, err := ProcessAvatarUpload([]byte{}, "image/png")
	if err == nil {
		t.Fatal("expected error for empty slice")
	}
}

func TestProcessAvatarUpload_invalidImage(t *testing.T) {
	_, _, _, _, _, err := ProcessAvatarUpload([]byte("not an image"), "image/png")
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestProcessAvatarUpload_squareImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			src.Set(x, y, color.NRGBA{R: 128, G: 64, B: 32, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	main, thumb, w, h, mime, err := ProcessAvatarUpload(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime=%q", mime)
	}
	if w != 200 || h != 200 {
		t.Fatalf("main size=%dx%d, want 200x200", w, h)
	}
	if len(main) == 0 {
		t.Fatal("expected main data")
	}
	if len(thumb) == 0 {
		t.Fatal("expected thumbnail data")
	}
}

func TestProcessAvatarUpload_largeSquareDownscaled(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	main, thumb, w, h, mime, err := ProcessAvatarUpload(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime=%q", mime)
	}
	if w != 512 || h != 512 {
		t.Fatalf("main size=%dx%d, want 512x512", w, h)
	}
	if len(main) == 0 || len(thumb) == 0 {
		t.Fatal("expected encoded payloads")
	}
}

func TestProcessAvatarUpload_jpegInput(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 60))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	main, thumb, w, h, mime, err := ProcessAvatarUpload(buf.Bytes(), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime=%q", mime)
	}
	if w != 60 || h != 60 {
		t.Fatalf("main size=%dx%d, want 60x60 (landscape cropped to square)", w, h)
	}
	if len(main) == 0 || len(thumb) == 0 {
		t.Fatal("expected encoded payloads")
	}
}

func TestProcessAvatarUpload_gifInput(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 80))
	var buf bytes.Buffer
	if err := gif.Encode(&buf, src, &gif.Options{NumColors: 256}); err != nil {
		t.Fatal(err)
	}
	main, thumb, w, h, mime, err := ProcessAvatarUpload(buf.Bytes(), "image/gif")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime=%q", mime)
	}
	if w != 80 || h != 80 {
		t.Fatalf("main size=%dx%d, want 80x80", w, h)
	}
	if len(main) == 0 || len(thumb) == 0 {
		t.Fatal("expected encoded payloads")
	}
}

func TestProcessAvatarUpload_portraitCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	main, _, w, h, _, err := ProcessAvatarUpload(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if w != 40 || h != 40 {
		t.Fatalf("main size=%dx%d, want 40x40 (portrait cropped to square)", w, h)
	}
	if len(main) == 0 {
		t.Fatal("expected main data")
	}
}
