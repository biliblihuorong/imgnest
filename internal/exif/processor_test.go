package exif

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
)

func TestArchiveFullMetadataAndScrubJPEG(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tiff := metadataTIFF()
	data := jpegMetadata(t, tiff)
	info := imaging.Info{Format: "jpeg", Orientation: 6}
	before, err := p.Extract(t.Context(), data, info)
	if err != nil {
		t.Fatal(err)
	}
	if before.Make != "Test Camera" || before.Model != "Test Model" || before.Lens != "Test Lens" || before.GPSLat == nil || before.GPSLng == nil || before.ISO != 100 {
		t.Fatalf("common metadata was not extracted: make=%q model=%q lens=%q ISO=%d", before.Make, before.Model, before.Lens, before.ISO)
	}
	var raw struct {
		Blocks []struct {
			Kind string
			Data []byte
		}
	}
	if err := json.Unmarshal(before.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Blocks) != 3 {
		t.Fatalf("raw blocks=%d, want EXIF and both XMP packets", len(raw.Blocks))
	}
	if !bytes.Contains(raw.Blocks[0].Data, []byte("opaque-unknown")) || !bytes.Contains(raw.Blocks[0].Data, []byte("maker-private")) {
		t.Fatal("opaque metadata was lost")
	}
	out, err := p.Scrub(t.Context(), data, "jpeg", "gps")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jpegScan(data), jpegScan(out)) {
		t.Fatal("JPEG compressed pixels changed")
	}
	for _, secret := range []string{"author-private", "serial-private", "maker-private", "private-xmp"} {
		if bytes.Contains(out, []byte(secret)) {
			t.Fatal("identity metadata survived scrub")
		}
	}
	after, err := p.Extract(t.Context(), out, info)
	if err != nil {
		t.Fatal(err)
	}
	if after.GPSLat != nil || after.GPSLng != nil || after.Make != before.Make || after.Model != before.Model || after.Orientation != 6 {
		t.Fatal("GPS scrub lost safe fields or retained GPS")
	}
	all, err := p.Scrub(t.Context(), data, "jpeg", "all")
	if err != nil {
		t.Fatal(err)
	}
	after, err = p.Extract(t.Context(), all, info)
	if err != nil {
		t.Fatal(err)
	}
	if after.Make != "" || after.Model != "" || after.Orientation != 6 || !bytes.Equal(jpegScan(data), jpegScan(all)) {
		t.Fatal("all scrub did not preserve only orientation and pixels")
	}
	if !bytes.Contains(data, []byte("author-private")) {
		t.Fatal("source was modified")
	}
}

// Vendor-private tags (IDs at or above 0xC000) inside embedded EXIF may hold
// device or owner identifiers beyond the named scrub list; the gps profile
// strips them from JPEGs, PNGs and WebPs without touching pixels.
func TestScrubStripsEmbeddedVendorTags(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	data := jpegMetadata(t, metadataTIFF())
	out, err := p.Scrub(t.Context(), data, "jpeg", "gps")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("opaque-unknown")) {
		t.Fatal("vendor-private tag survived the gps scrub")
	}
	if !bytes.Equal(jpegScan(data), jpegScan(out)) {
		t.Fatal("JPEG compressed pixels changed")
	}
	if _, err := p.Extract(t.Context(), out, imaging.Info{Format: "jpeg", Orientation: 6}); err != nil {
		t.Fatalf("scrubbed EXIF no longer parses: %v", err)
	}
}

func TestPNGAndWebPMetadataContainers(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"png", "webp"} {
		t.Run(format, func(t *testing.T) {
			var data, pixels []byte
			if format == "png" {
				data = pngMetadata(t, metadataTIFF())
				pixels = pngPayload(data, "IDAT")
			} else {
				data = webpMetadata(metadataTIFF())
				pixels = webpPayload(data, "VP8 ")
			}
			before, err := p.Extract(t.Context(), data, imaging.Info{Format: format, Orientation: 6})
			if err != nil {
				t.Fatal(err)
			}
			if before.GPSLat == nil {
				t.Fatal("GPS missing before scrub")
			}
			out, err := p.Scrub(t.Context(), data, format, "gps")
			if err != nil {
				t.Fatal(err)
			}
			after, err := p.Extract(t.Context(), out, imaging.Info{Format: format, Orientation: 6})
			if err != nil {
				t.Fatal(err)
			}
			if after.GPSLat != nil || after.Make != before.Make {
				t.Fatal("privacy or safe metadata contract failed")
			}
			if format == "png" {
				if !bytes.Equal(pixels, pngPayload(out, "IDAT")) {
					t.Fatal("PNG pixels changed")
				}
			} else {
				if !bytes.Equal(pixels, webpPayload(out, "VP8 ")) || out[20]&4 != 0 {
					t.Fatal("WebP pixels or XMP flag incorrect")
				}
			}
		})
	}
}

func TestMetadataBoundsCancellationAndModes(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewProcessor(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("constructor ignored cancellation")
	}
	if _, err := p.Extract(ctx, nil, imaging.Info{}); !errors.Is(err, context.Canceled) {
		t.Fatal("extract ignored cancellation")
	}
	if _, err := p.Scrub(ctx, nil, "jpeg", "none"); !errors.Is(err, context.Canceled) {
		t.Fatal("scrub ignored cancellation")
	}
	data := jpegMetadata(t, metadataTIFF())
	out, err := p.Scrub(t.Context(), data, "jpeg", "none")
	if err != nil || !bytes.Equal(out, data) {
		t.Fatal("none modified original")
	}
	if _, err := p.Scrub(t.Context(), data, "jpeg", "unknown"); err == nil {
		t.Fatal("unknown scrub mode accepted")
	}
	for _, format := range []string{"heic", "avif", "unknown"} {
		if _, err := p.Scrub(t.Context(), data, format, "gps"); err == nil {
			t.Fatal("unsupported original scrub accepted")
		}
	}
	bad := metadataTIFF()
	binary.LittleEndian.PutUint32(bad[4:8], 0xfffffff0)
	if _, err := p.Scrub(t.Context(), jpegMetadata(t, bad), "jpeg", "gps"); err == nil {
		t.Fatal("out-of-bounds TIFF accepted")
	}
	cycle := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0, 0, 8, 0, 0, 0}
	if _, err := p.Extract(t.Context(), jpegMetadata(t, cycle), imaging.Info{Format: "jpeg"}); err == nil {
		t.Fatal("cyclic TIFF accepted")
	}
	if _, err := p.Extract(t.Context(), pngMetadata(t, metadataTIFF())[:25], imaging.Info{Format: "png"}); err == nil {
		t.Fatal("truncated container accepted")
	}
}

type fixtureTag struct {
	id, kind uint16
	count    uint32
	data     []byte
}

func metadataTIFF() []byte {
	ifd0 := []fixtureTag{{0x10f, 2, 12, []byte("Test Camera\x00")}, {0x110, 2, 11, []byte("Test Model\x00")}, {0x112, 3, 1, []byte{6, 0}}, {0x13b, 2, 15, []byte("author-private\x00")}, {0x8769, 4, 1, nil}, {0x8825, 4, 1, nil}, {0xc7aa, 7, 14, []byte("opaque-unknown")}}
	exif := []fixtureTag{{0xa434, 2, 10, []byte("Test Lens\x00")}, {0xa431, 2, 15, []byte("serial-private\x00")}, {0x927c, 7, 13, []byte("maker-private")}, {0x8827, 3, 1, []byte{100, 0}}}
	gps := []fixtureTag{{1, 2, 2, []byte{'N', 0}}, {2, 5, 3, rationals(1, 2, 3)}, {3, 2, 2, []byte{'E', 0}}, {4, 5, 3, rationals(4, 5, 6)}, {5, 1, 1, []byte{0}}, {6, 5, 1, rationals(7)}}
	exifOffset := 8 + 2 + len(ifd0)*12 + 4
	gpsOffset := exifOffset + 2 + len(exif)*12 + 4
	dataOffset := gpsOffset + 2 + len(gps)*12 + 4
	return buildFixtureTIFF(ifd0, exif, gps, exifOffset, gpsOffset, dataOffset)
}

func buildFixtureTIFF(ifd0, exif, gps []fixtureTag, exifOffset, gpsOffset, dataOffset int) []byte {
	b := make([]byte, dataOffset)
	copy(b, []byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	for dirIndex, tags := range [][]fixtureTag{ifd0, exif, gps} {
		off := []int{8, exifOffset, gpsOffset}[dirIndex]
		if len(tags) > 4096 {
			panic("synthetic fixture directory too large")
		}
		binary.LittleEndian.PutUint16(b[off:off+2], uint16(len(tags)&65535))
		for i, tag := range tags {
			entry := off + 2 + i*12
			binary.LittleEndian.PutUint16(b[entry:], tag.id)
			binary.LittleEndian.PutUint16(b[entry+2:], tag.kind)
			binary.LittleEndian.PutUint32(b[entry+4:], tag.count)
			switch tag.id {
			case 0x8769:
				binary.LittleEndian.PutUint32(b[entry+8:], uint32(exifOffset&0xffffff))
			case 0x8825:
				binary.LittleEndian.PutUint32(b[entry+8:], uint32(gpsOffset&0xffffff))
			default:
				if len(tag.data) <= 4 {
					copy(b[entry+8:entry+12], tag.data)
				} else {
					binary.LittleEndian.PutUint32(b[entry+8:], uint32(len(b)&0xffffff))
					b = append(b, tag.data...)
				}
			}
		}
	}
	return b
}

func rationals(values ...uint32) []byte {
	b := make([]byte, len(values)*8)
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[i*8:], v)
		binary.LittleEndian.PutUint32(b[i*8+4:], 1)
	}
	return b
}
func jpegMetadata(t *testing.T, tiff []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3)), nil); err != nil {
		t.Fatal(err)
	}
	out := append([]byte{}, b.Bytes()[:2]...)
	for _, payload := range [][]byte{append([]byte("Exif\x00\x00"), tiff...), []byte("http://ns.adobe.com/xap/1.0/\x00<private-xmp/>"), []byte("http://ns.adobe.com/xmp/extension/\x00private-xmp")} {
		out = append(out, 0xff, 0xe1, byte(((len(payload)+2)>>8)&255), byte((len(payload)+2)&255))
		out = append(out, payload...)
	}
	return append(out, b.Bytes()[2:]...)
}
func jpegScan(data []byte) []byte {
	i := bytes.Index(data, []byte{0xff, 0xda})
	if i < 0 {
		return nil
	}
	return data[i:]
}
func pngMetadata(t *testing.T, tiff []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	out := append([]byte{}, b.Bytes()[:33]...)
	out = append(out, pngChunk("eXIf", tiff)...)
	out = append(out, pngChunk("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00<private-xmp/>"))...)
	return append(out, b.Bytes()[33:]...)
}
func pngChunk(kind string, data []byte) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, uint32(uint64(len(data))&0xffffffff))
	copy(b[4:], kind)
	b = append(b, data...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(b[4:]))
	return append(b, crc...)
}
func pngPayload(data []byte, kind string) []byte {
	out := []byte{}
	for pos := 8; pos+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		if pos+12+n > len(data) {
			return nil
		}
		if string(data[pos+4:pos+8]) == kind {
			out = append(out, data[pos+8:pos+8+n]...)
		}
		pos += 12 + n
	}
	return out
}
func webpMetadata(tiff []byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBP")
	b = append(b, riffChunk("VP8X", []byte{12, 0, 0, 0, 1, 0, 0, 2, 0, 0})...)
	b = append(b, riffChunk("VP8 ", []byte("compressed-pixels"))...)
	b = append(b, riffChunk("EXIF", tiff)...)
	b = append(b, riffChunk("XMP ", []byte("private-xmp"))...)
	binary.LittleEndian.PutUint32(b[4:8], uint32((uint64(len(b))-8)&0xffffffff))
	return b
}
func riffChunk(kind string, data []byte) []byte {
	b := make([]byte, 8)
	copy(b, kind)
	binary.LittleEndian.PutUint32(b[4:], uint32(uint64(len(data))&0xffffffff))
	b = append(b, data...)
	if len(data)%2 != 0 {
		b = append(b, 0)
	}
	return b
}
func webpPayload(data []byte, kind string) []byte {
	for pos := 12; pos+8 <= len(data); {
		n := int(binary.LittleEndian.Uint32(data[pos+4:]))
		if pos+8+n > len(data) {
			return nil
		}
		if string(data[pos:pos+4]) == kind {
			return data[pos+8 : pos+8+n]
		}
		pos += 8 + n + (n % 2)
	}
	return nil
}
