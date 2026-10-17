package exif

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/jpeg"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
)

// A photo past the old 20 MiB source cap is archived and scrubbed like any
// other: only the metadata budget, not the file size, bounds the archive.
func TestLargeJPEGIsArchivedAndScrubbed(t *testing.T) {
	noise := image.NewRGBA(image.Rect(0, 0, 3600, 3600))
	if _, err := rand.Read(noise.Pix); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, noise, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	tiff := metadataTIFF()
	payload := append([]byte("Exif\x00\x00"), tiff...)
	data := append([]byte{}, encoded.Bytes()[:2]...)
	data = append(data, 0xff, 0xe1, byte(((len(payload)+2)>>8)&255), byte((len(payload)+2)&255))
	data = append(data, payload...)
	data = append(data, encoded.Bytes()[2:]...)
	if len(data) <= maxMetadataBytes {
		t.Fatalf("fixture is only %d bytes", len(data))
	}
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	info := imaging.Info{Format: "jpeg", Orientation: 1}
	before, err := p.Extract(t.Context(), data, info)
	if err != nil || before.GPSLat == nil || len(before.Raw) == 0 || len(before.Raw) > maxMetadataBytes {
		t.Fatalf("large JPEG archive = %d bytes, gps=%v, err=%v", len(before.Raw), before.GPSLat, err)
	}
	out, err := p.Scrub(t.Context(), data, "jpeg", "gps")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jpegScan(data), jpegScan(out)) {
		t.Fatal("JPEG compressed pixels changed")
	}
	after, err := p.Extract(t.Context(), out, info)
	if err != nil || after.GPSLat != nil || after.Make != before.Make {
		t.Fatalf("scrubbed large JPEG = %+v, err=%v", after, err)
	}
}
