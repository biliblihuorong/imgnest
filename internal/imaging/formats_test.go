package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/cshum/vipsgen/vips"
)

func TestProcessMetadataFlagsAndAlpha(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := vips.ProfileLoad("srgb")
	if err != nil {
		t.Fatal(err)
	}
	data := jpegFixture(t, 10, 10, 1)
	segments := []byte{}
	for i, payload := range [][]byte{append([]byte("ICC_PROFILE\x00\x01\x01"), profile...), []byte("http://ns.adobe.com/xap/1.0/\x00<private-xmp/>")} {
		marker := byte(0xe2)
		if i == 1 {
			marker = 0xe1
		}
		if len(payload) > 65533 {
			t.Fatal("synthetic JPEG segment too large")
		}
		segments = append(segments, 0xff, marker, byte(((len(payload)+2)>>8)&255), byte((len(payload)+2)&255))
		segments = append(segments, payload...)
	}
	data = append(append(append([]byte{}, data[:2]...), segments...), data[2:]...)
	r, err := p.Process(t.Context(), data, Options{WebPMode: "both", Quality: 80, Effort: 4, ThumbEnabled: true, ThumbSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		icc  bool
	}{{"webp", r.WebP, true}, {"thumbnail", r.Thumbnail, false}} {
		image, err := vips.NewImageFromBuffer(tc.data, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range image.GetFields() {
			if strings.HasPrefix(field, "exif") || field == "xmp-data" {
				image.Close()
				t.Fatal("derivative contains private metadata")
			}
		}
		actual, exists := image.GetICCProfile()
		image.Close()
		if exists != tc.icc {
			t.Fatalf("%s ICC retention=%v", tc.name, exists)
		}
		if tc.icc && !bytes.Equal(profile, actual) {
			t.Fatal("valid RGB ICC profile changed")
		}
	}
	r, err = p.Process(t.Context(), pngFixture(t, 2, 3), Options{WebPMode: "both", Quality: 80, Effort: 4, Lossless: true})
	if err != nil {
		t.Fatal(err)
	}
	image, err := vips.NewImageFromBuffer(r.WebP, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	if !image.HasAlpha() {
		t.Fatal("PNG transparency was lost")
	}
}

func TestProbeSyntheticAdditionalFormats(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	image, err := vips.NewImageFromBuffer(pngFixture(t, 64, 32), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	tiff, err := image.TiffsaveBuffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	heicOpts := vips.DefaultHeifsaveBufferOptions()
	heicOpts.Compression = vips.HeifCompressionHevc
	heicOpts.Bitdepth = 8
	heicOpts.Q = 80
	heicOpts.Effort = 1
	heicOpts.Keep = vips.KeepNone
	heic, err := image.HeifsaveBuffer(heicOpts)
	if err != nil {
		t.Fatal(err)
	}
	avifOpts := vips.DefaultHeifsaveBufferOptions()
	avifOpts.Compression = vips.HeifCompressionAv1
	avifOpts.Bitdepth = 8
	avifOpts.Q = 80
	avifOpts.Effort = 1
	avifOpts.Keep = vips.KeepNone
	avif, err := image.HeifsaveBuffer(avifOpts)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		data          []byte
		width, height int
	}{{"bmp", bmpFixture(), 2, 3}, {"tiff", tiff, 64, 32}, {"heic", heic, 64, 32}, {"avif", avif, 64, 32}} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := p.Probe(t.Context(), tc.data)
			if err != nil {
				native, nativeErr := vips.NewImageFromBuffer(tc.data, nil)
				if native != nil {
					native.Close()
				}
				t.Fatalf("native fixture probe: %v; direct loader: %v", err, nativeErr)
			}
			if info.Format != tc.name || info.Width != tc.width || info.Height != tc.height || info.LoadedFrames != 1 {
				t.Fatalf("synthetic format info=%+v", info)
			}
			if _, err := p.Process(t.Context(), tc.data, Options{WebPMode: "both", Quality: 80, Effort: 4, ThumbEnabled: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := image.Colourspace(vips.InterpretationCmyk, nil); err != nil {
		t.Fatal(err)
	}
	cmyk, err := image.JpegsaveBuffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Process(t.Context(), cmyk, Options{WebPMode: "both", Quality: 80, Effort: 4, ThumbEnabled: true}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessorValidationSkipLargerAndQueue(t *testing.T) {
	if _, err := NewProcessor(t.Context(), 0, 1); err == nil {
		t.Fatal("invalid pixel budget accepted")
	}
	if _, err := NewProcessor(t.Context(), 1, 0); err == nil {
		t.Fatal("invalid concurrency accepted")
	}
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []Options{{WebPMode: "both", Quality: -1}, {WebPMode: "both", Quality: 101}, {WebPMode: "both", Effort: 7}, {WebPMode: "both", MaxWidth: -1}, {WebPMode: "both", ThumbSize: -1}} {
		if _, err := p.Process(t.Context(), pngFixture(t, 1, 1), opts); !errors.Is(err, ErrInvalidOptions) {
			t.Fatal("invalid options accepted")
		}
	}
	data := gifFixture(t)
	base, err := p.Process(t.Context(), data, Options{WebPMode: "both", Quality: 80, Effort: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.WebP) <= len(data) {
		t.Fatal("synthetic animation does not exercise larger derivative")
	}
	r, err := p.Process(t.Context(), data, Options{WebPMode: "both", Quality: 80, Effort: 4, SkipIfLarger: true})
	if err != nil || len(r.WebP) != 0 {
		t.Fatal("larger both derivative was not omitted")
	}
	r, err = p.Process(t.Context(), data, Options{WebPMode: "webp_only", Quality: 80, Effort: 4, SkipIfLarger: true})
	if err != nil || len(r.WebP) == 0 {
		t.Fatal("webp_only incorrectly omitted its sole output")
	}
	p.slots <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := p.Probe(ctx, data); result <- err }()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("queued image ignored cancellation")
	}
	p.release()
	if len(p.slots) != 0 {
		t.Fatal("processing permit leaked")
	}
}

func bmpFixture() []byte {
	data := make([]byte, 78)
	copy(data, "BM")
	binary.LittleEndian.PutUint32(data[2:], 78)
	binary.LittleEndian.PutUint32(data[10:], 54)
	binary.LittleEndian.PutUint32(data[14:], 40)
	binary.LittleEndian.PutUint32(data[18:], 2)
	binary.LittleEndian.PutUint32(data[22:], 3)
	binary.LittleEndian.PutUint16(data[26:], 1)
	binary.LittleEndian.PutUint16(data[28:], 24)
	binary.LittleEndian.PutUint32(data[34:], 24)
	for i := 54; i < len(data); i++ {
		data[i] = byte(i)
	}
	return data
}
