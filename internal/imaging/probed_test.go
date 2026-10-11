package imaging

import "testing"

// A probed buffer is processed without a second validation decode, and a
// Probed value for another format is ignored rather than trusted.
func TestProcessReusesMatchingProbe(t *testing.T) {
	p, err := NewProcessor(t.Context(), 100_000_000, 2)
	if err != nil {
		t.Fatal(err)
	}
	data := pngFixture(t, 4, 3)
	info, err := p.Probe(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	marked := info
	marked.LoadedFrames = 7 // proves the supplied value is the one used
	result, err := p.Process(t.Context(), data, Options{WebPMode: "both", ThumbEnabled: true, Probed: &marked})
	if err != nil {
		t.Fatal(err)
	}
	if result.Info.LoadedFrames != 7 || len(result.WebP) == 0 || len(result.Thumbnail) == 0 {
		t.Fatalf("probe not reused: %+v", result.Info)
	}
	foreign := Info{Format: "jpeg", Width: 1, Height: 1, LoadedFrames: 9}
	result, err = p.Process(t.Context(), data, Options{WebPMode: "none", Probed: &foreign})
	if err != nil || result.Info.Format != "png" || result.Info.LoadedFrames != 1 {
		t.Fatalf("mismatched probe trusted: %+v err=%v", result.Info, err)
	}
}
