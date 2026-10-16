package imaging

import (
	"testing"

	"github.com/cshum/vipsgen/vips"
)

func TestVariantsKeepRatioShrinkOnlyAndActualSizes(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	widths := []int{400, 800, 1600}
	for _, tc := range []struct {
		name string
		data []byte
		want [][2]int
	}{
		{name: "landscape", data: pngFixture(t, 2000, 1000), want: [][2]int{{400, 200}, {800, 400}, {1600, 800}}},
		{name: "portrait", data: pngFixture(t, 1000, 2001), want: [][2]int{{400, 800}, {800, 1601}}},
		{name: "small", data: pngFixture(t, 300, 200), want: nil},
		{name: "exact width is not duplicated", data: pngFixture(t, 800, 600), want: [][2]int{{400, 300}}},
		{name: "orientation 6", data: jpegFixture(t, 1200, 600, 6), want: [][2]int{{400, 800}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			variants, err := p.Variants(t.Context(), tc.data, VariantOptions{Widths: widths, Quality: 80, Effort: 4})
			if err != nil {
				t.Fatal(err)
			}
			if len(variants) != len(tc.want) {
				t.Fatalf("got %d variants, want %d", len(variants), len(tc.want))
			}
			for i, variant := range variants {
				if variant.Width != tc.want[i][0] || variant.Height != tc.want[i][1] {
					t.Fatalf("variant %d is %dx%d, want %v", i, variant.Width, variant.Height, tc.want[i])
				}
				info, err := p.Probe(t.Context(), variant.Data)
				if err != nil {
					t.Fatal(err)
				}
				if info.Format != "webp" || info.Width != variant.Width || info.Height != variant.Height || info.LoadedFrames != 1 || info.Orientation != 1 {
					t.Fatalf("reported size differs from the encoded file: %+v vs %+v", info, variant)
				}
			}
		})
	}
}

func TestVariantsKeepAlphaAndSkipAnimation(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	variants, err := p.Variants(t.Context(), pngFixture(t, 900, 300), VariantOptions{Widths: []int{400}})
	if err != nil || len(variants) != 1 {
		t.Fatalf("transparent PNG variants: %v %d", err, len(variants))
	}
	img, err := vips.NewImageFromBuffer(variants[0].Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	if !img.HasAlpha() {
		t.Fatal("transparent variant lost its alpha channel")
	}
	animated, err := p.Variants(t.Context(), gifFixture(t), VariantOptions{Widths: []int{1}})
	if err != nil || len(animated) != 0 {
		t.Fatalf("animation produced static variants: %v %d", err, len(animated))
	}
	if _, err := p.Variants(t.Context(), pngFixture(t, 10, 10), VariantOptions{Widths: []int{0}}); err == nil {
		t.Fatal("invalid width accepted")
	}
}
