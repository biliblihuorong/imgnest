package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestProbeVisualOrientation(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                  string
		data                  []byte
		format                string
		width, height, frames int
	}{
		{name: "alpha PNG", data: pngFixture(t, 2, 3), format: "png", width: 2, height: 3, frames: 1},
		{name: "oriented JPEG", data: jpegFixture(t, 2, 3, 6), format: "jpeg", width: 3, height: 2, frames: 1},
		{name: "animated GIF", data: gifFixture(t), format: "gif", width: 2, height: 3, frames: 2},
		{name: "one pixel", data: pngFixture(t, 1, 1), format: "png", width: 1, height: 1, frames: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := p.Probe(t.Context(), tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if info.Format != tc.format || info.Width != tc.width || info.Height != tc.height || info.LoadedFrames != tc.frames {
				t.Fatalf("unexpected info: %+v", info)
			}
		})
	}
}

func TestProcessPreservesAnimationAndMakesStaticThumbnail(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Process(t.Context(), gifFixture(t), Options{WebPMode: "both", Quality: 80, Effort: 4, ThumbEnabled: true, ThumbSize: 400})
	if err != nil {
		t.Fatal(err)
	}
	webp, err := p.Probe(t.Context(), result.WebP)
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := p.Probe(t.Context(), result.Thumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if webp.LoadedFrames != 2 || thumb.LoadedFrames != 1 || thumb.Width != 2 || thumb.Height != 3 {
		t.Fatal("animation or no-upscale thumbnail contract failed")
	}
	if webp.Ext != "webp" || webp.MIME != "image/webp" {
		t.Fatal("generated format contract failed")
	}
}

func TestProcessModesAndResize(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	data := pngFixture(t, 20, 10)
	for _, mode := range []string{"both", "webp_only", "none"} {
		r, err := p.Process(t.Context(), data, Options{WebPMode: mode, Quality: 80, Effort: 4, MaxWidth: 8, MaxHeight: 8, ThumbEnabled: true, ThumbSize: 4})
		if err != nil {
			t.Fatal(err)
		}
		if mode == "none" {
			if len(r.WebP) != 0 {
				t.Fatal("none produced WebP")
			}
		} else {
			info, err := p.Probe(t.Context(), r.WebP)
			if err != nil {
				t.Fatal(err)
			}
			if info.Width != 8 || info.Height != 4 {
				t.Fatalf("resize info=%+v", info)
			}
		}
		info, err := p.Probe(t.Context(), r.Thumbnail)
		if err != nil {
			t.Fatal(err)
		}
		if info.Width != 4 || info.Height != 2 {
			t.Fatalf("thumbnail info=%+v", info)
		}
	}
	first, err := p.Process(t.Context(), data, Options{WebPMode: "webp_only", Quality: 80, Effort: 4})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Process(t.Context(), first.WebP, Options{WebPMode: "both", Quality: 80, Effort: 4, ThumbEnabled: true, ThumbSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.WebP) != 0 || len(r.Thumbnail) == 0 {
		t.Fatal("source WebP was reencoded or thumbnail missing")
	}
}

func TestProbeRejectsMalformedAndPixelBombs(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	bomb := pngFixture(t, 1, 1)
	binary.BigEndian.PutUint32(bomb[16:20], 100_001)
	binary.BigEndian.PutUint32(bomb[20:24], 100_001)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	for _, data := range [][]byte{nil, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"), pngFixture(t, 1, 1)[:30], []byte("not an image"), bomb} {
		if _, err := p.Probe(t.Context(), data); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	if _, err := p.Process(t.Context(), pngFixture(t, 1, 1), Options{WebPMode: "invalid"}); err == nil {
		t.Fatal("invalid options accepted")
	}
}

func TestProcessorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewProcessor(ctx, 100_000_000, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("constructor ignored cancellation")
	}
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Probe(ctx, pngFixture(t, 1, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal("probe ignored cancellation")
	}
	if _, err := p.Process(ctx, pngFixture(t, 1, 1), Options{WebPMode: "none"}); !errors.Is(err, context.Canceled) {
		t.Fatal("process ignored cancellation")
	}
}

func TestProbeRejectsTruncatedPixels(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	jpeg := jpegFixture(t, 10, 10, 1)
	png := pngFixture(t, 10, 10)
	gif := gifFixture(t)
	for _, data := range [][]byte{jpeg[:len(jpeg)-2], png[:len(png)-12], gif[:len(gif)-1]} {
		if _, err := p.Probe(t.Context(), data); err == nil {
			t.Fatal("truncated pixels or missing image terminator accepted")
		}
	}
}

func pngFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 70, A: uint8(80 + x%150)})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifFixture(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	a := image.NewPaletted(image.Rect(0, 0, 2, 3), pal)
	b := image.NewPaletted(a.Bounds(), pal)
	b.SetColorIndex(0, 0, 1)
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, &gif.GIF{Image: []*image.Paletted{a, b}, Delay: []int{4, 8}, LoopCount: 3}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func jpegFixture(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	if orientation < 1 || orientation > 8 {
		t.Fatal("invalid synthetic orientation")
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(orientation & 255), 0, 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	if len(payload) > 65533 {
		t.Fatal("synthetic JPEG segment too large")
	}
	segment := []byte{0xff, 0xe1, byte(((len(payload) + 2) >> 8) & 255), byte((len(payload) + 2) & 255)}
	segment = append(segment, payload...)
	result := append([]byte{}, out.Bytes()[:2]...)
	result = append(result, segment...)
	return append(result, out.Bytes()[2:]...)
}
