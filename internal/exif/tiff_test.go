package exif

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/cshum/vipsgen/vips"
)

// Distinct TIFF offsets may reference almost the same metadata bytes. Archiving
// each entry independently must not multiply the source-size memory budget.
func TestTIFFArchiveBoundsOverlappingValuesBySourceSize(t *testing.T) {
	data, _ := onePixelMetadataTIFF(t, 100, 65536)
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ip, err := imaging.NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	info, err := ip.Probe(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.Extract(t.Context(), data, info)
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	archivedBytes := 0
	for _, block := range archive.Blocks {
		archivedBytes += len(block.Data)
	}
	if archivedBytes > len(data) {
		t.Fatalf("archived %d bytes from %d source bytes", archivedBytes, len(data))
	}
	assertTIFFArchivedRange(t, archive, data, 0, len(data)-1)
	if archive.CaptureMode != "metadata-blocks" {
		t.Fatal("bounded reachable metadata unexpectedly used opaque-layout fallback")
	}
	for _, block := range archive.Blocks {
		if block.Offset+len(block.Data) > len(data)-1 {
			t.Fatal("metadata archive unnecessarily retained TIFF pixels")
		}
	}
	if len(meta.Raw) > 2*len(data) {
		t.Fatalf("raw archive exceeds source-size budget: raw=%d source=%d", len(meta.Raw), len(data))
	}
}

// Deduplicating only by offset loses the tail of a longer second value.
func TestTIFFArchiveRetainsLongerValueAtSameOffset(t *testing.T) {
	data, valuesOffset := onePixelMetadataTIFF(t, 2, 32)
	binary.LittleEndian.PutUint32(data[10+10*12+4:], 8)
	binary.LittleEndian.PutUint32(data[10+11*12+8:], fixtureOffset(t, valuesOffset))
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.Extract(t.Context(), data, imaging.Info{Format: "tiff"})
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	assertTIFFArchivedRange(t, archive, data, valuesOffset, valuesOffset+32)
}

// A reachable value can share bytes with the TIFF header or IFD itself.
// Budgeting only value/value overlaps would still archive these bytes twice.
func TestTIFFArchiveCoalescesValuesOverlappingHeaderAndIFD(t *testing.T) {
	data, valuesOffset := onePixelMetadataTIFF(t, 2, 64)
	binary.LittleEndian.PutUint32(data[10+10*12+8:], 0)
	binary.LittleEndian.PutUint32(data[10+11*12+8:], 10)
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.Extract(t.Context(), data, imaging.Info{Format: "tiff"})
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	assertTIFFArchivedRange(t, archive, data, 0, valuesOffset)
	archivedBytes := 0
	for _, block := range archive.Blocks {
		archivedBytes += len(block.Data)
	}
	if archivedBytes != valuesOffset {
		t.Fatalf("archived %d bytes for a %d-byte metadata union", archivedBytes, valuesOffset)
	}
}

func TestTIFFGPSRemovesCameraSerialNumberWithoutChangingPixels(t *testing.T) {
	data, _ := onePixelMetadataTIFF(t, 1, 32)
	serialPos := 10 + 10*12
	binary.LittleEndian.PutUint16(data[serialPos:], 0xc62f)
	binary.LittleEndian.PutUint16(data[serialPos+2:], 2)
	serialOffset := int(binary.LittleEndian.Uint32(data[serialPos+8:]))
	clear(data[serialOffset : serialOffset+32])
	copy(data[serialOffset:], "camera-serial-private")
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ip, err := imaging.NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	info, err := ip.Probe(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.Extract(t.Context(), data, info)
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	assertTIFFArchivedRange(t, archive, data, serialOffset, serialOffset+32)
	for _, mode := range []string{"gps", "all"} {
		t.Run(mode, func(t *testing.T) {
			out, err := p.Scrub(t.Context(), data, "tiff", mode)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(out, []byte("camera-serial-private")) {
				t.Fatal("camera serial number survived cloud-original scrub")
			}
			if _, err := ip.Probe(t.Context(), out); err != nil {
				t.Fatalf("scrubbed TIFF no longer loads: %v", err)
			}
			if out[len(out)-1] != 0x7b {
				t.Fatal("stored TIFF pixel byte changed")
			}
			assertDecodedTIFFPixel(t, data)
			assertDecodedTIFFPixel(t, out)
		})
	}
	if !bytes.Contains(data, []byte("camera-serial-private")) {
		t.Fatal("cloud-original scrub modified the upload source")
	}
}

// DNG uses high-numbered tags for image interpretation as well as identity.
// GPS scrubbing must keep the safe tags while removing CameraSerialNumber.
func TestTIFFGPSPreservesDNGStructureTags(t *testing.T) {
	data, _ := onePixelMetadataTIFF(t, 3, 32)
	binary.LittleEndian.PutUint16(data[10+10*12:], 0xc612) // DNGVersion
	binary.LittleEndian.PutUint16(data[10+10*12+2:], 1)
	binary.LittleEndian.PutUint32(data[10+10*12+4:], 4)
	copy(data[10+10*12+8:], []byte{1, 4, 0, 0})
	binary.LittleEndian.PutUint16(data[10+11*12:], 0xc614) // UniqueCameraModel
	binary.LittleEndian.PutUint16(data[10+11*12+2:], 2)
	modelOffset := int(binary.LittleEndian.Uint32(data[10+11*12+8:]))
	clear(data[modelOffset : modelOffset+32])
	copy(data[modelOffset:], "Test Camera Model")
	// Place the serial in its own payload, not the safe model's shared range.
	serialOffset := len(data)
	data = append(data, []byte("camera-serial-private\x00")...)
	binary.LittleEndian.PutUint16(data[10+12*12:], 0xc62f)
	binary.LittleEndian.PutUint16(data[10+12*12+2:], 2)
	binary.LittleEndian.PutUint32(data[10+12*12+4:], 22)
	binary.LittleEndian.PutUint32(data[10+12*12+8:], fixtureOffset(t, serialOffset))
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Scrub(t.Context(), data, "tiff", "gps")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseTIFF(out)
	if err != nil {
		t.Fatal(err)
	}
	versionFound, modelFound := false, false
	for _, dir := range doc.dirs {
		for _, entry := range dir.entries {
			switch entry.id {
			case 0xc612:
				versionFound = bytes.Equal(out[entry.offset:entry.offset+entry.size], []byte{1, 4, 0, 0})
			case 0xc614:
				modelFound = bytes.Equal(out[entry.offset:entry.offset+17], []byte("Test Camera Model"))
			}
		}
	}
	if !versionFound || !modelFound {
		t.Fatal("GPS scrub removed or changed safe DNG structure tags")
	}
	if bytes.Contains(out, []byte("camera-serial-private")) {
		t.Fatal("camera serial number survived DNG cloud-original scrub")
	}
}

func TestTIFFGPSRejectsCameraSerialAliasingPaletteColors(t *testing.T) {
	data, palette := onePixelMetadataTIFF(t, 2, 1536)
	binary.LittleEndian.PutUint16(data[10+4*12+8:], 3) // Palette photometric interpretation.
	mapPos := 10 + 10*12
	binary.LittleEndian.PutUint16(data[mapPos:], 0x140)
	binary.LittleEndian.PutUint16(data[mapPos+2:], 3)
	binary.LittleEndian.PutUint32(data[mapPos+4:], 768)
	binary.LittleEndian.PutUint32(data[mapPos+8:], fixtureOffset(t, palette))
	serialPos := 10 + 11*12
	binary.LittleEndian.PutUint16(data[serialPos:], 0xc62f)
	binary.LittleEndian.PutUint16(data[serialPos+2:], 2)
	binary.LittleEndian.PutUint32(data[serialPos+4:], 22)
	binary.LittleEndian.PutUint32(data[serialPos+8:], fixtureOffset(t, palette))
	clear(data[palette : palette+1536])
	copy(data[palette:], "camera-serial-private\x00")
	data[len(data)-1] = 0 // Palette index zero decodes to red=0x61.
	original := bytes.Clone(data)
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ip, err := imaging.NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	info, err := ip.Probe(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Extract(t.Context(), data, info); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"gps", "all"} {
		t.Run(mode, func(t *testing.T) {
			out, err := p.Scrub(t.Context(), data, "tiff", mode)
			if !errors.Is(err, ErrInvalidMetadata) || out != nil {
				t.Fatal("scrub accepted camera serial bytes shared with retained palette colors")
			}
		})
	}
	if !bytes.Equal(data, original) {
		t.Fatal("rejected scrub modified the source")
	}
	img, err := vips.NewImageFromBuffer(data, &vips.LoadOptions{FailOn: vips.FailOnError})
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	pixels, err := img.WriteToMemory()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pixels, []byte{0x61, 0, 0}) {
		t.Fatalf("source palette pixels=%x, want 610000", pixels)
	}
}

func TestTIFFScrubRejectsSerialAliasingRetainedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		keptTag  uint16
		mode     string
		crossIFD bool
	}{
		{name: "camera model", keptTag: 0x110, mode: "gps"},
		{name: "ICC gps", keptTag: 0x8773, mode: "gps"},
		{name: "ICC all", keptTag: 0x8773, mode: "all"},
		{name: "ICC across IFD", keptTag: 0x8773, mode: "gps", crossIFD: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, shared := onePixelMetadataTIFF(t, 2, 32)
			keptPos := 10 + 10*12
			binary.LittleEndian.PutUint16(data[keptPos:], tc.keptTag)
			binary.LittleEndian.PutUint16(data[keptPos+2:], 2)
			serialPos := 10 + 11*12
			binary.LittleEndian.PutUint16(data[serialPos:], 0xc62f)
			binary.LittleEndian.PutUint16(data[serialPos+2:], 2)
			binary.LittleEndian.PutUint32(data[serialPos+8:], fixtureOffset(t, shared))
			clear(data[shared : shared+32])
			copy(data[shared:], "camera-serial-private\x00")
			if tc.crossIFD {
				childOffset := len(data)
				data = append(data, make([]byte, 18)...)
				binary.LittleEndian.PutUint16(data[childOffset:], 1)
				copy(data[childOffset+2:childOffset+14], data[keptPos:keptPos+12])
				binary.LittleEndian.PutUint16(data[keptPos:], 0x8769)
				binary.LittleEndian.PutUint16(data[keptPos+2:], 4)
				binary.LittleEndian.PutUint32(data[keptPos+4:], 1)
				binary.LittleEndian.PutUint32(data[keptPos+8:], fixtureOffset(t, childOffset))
			}
			original := bytes.Clone(data)
			p, err := NewProcessor(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			out, err := p.Scrub(t.Context(), data, "tiff", tc.mode)
			if !errors.Is(err, ErrInvalidMetadata) || out != nil {
				t.Fatal("scrub accepted identity bytes shared with retained metadata")
			}
			if !bytes.Equal(data, original) {
				t.Fatal("rejected scrub modified the source")
			}
		})
	}
}

// Directory compaction changes counts and entry bytes even when the removed
// identity payload is separate from a retained palette. Those aliases must
// fail closed instead of silently changing the cloud original's pixels.
func TestTIFFScrubRejectsPaletteAliasingRewrittenDirectory(t *testing.T) {
	for _, tc := range []struct {
		name          string
		paletteOffset int
		pixels        []byte
	}{
		{name: "directory count", paletteOffset: 7, pixels: []byte{12, 4, 0}},
		{name: "compacted entry", paletteOffset: 131, pixels: []byte{2, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := paletteDirectoryAliasTIFF(t, tc.paletteOffset, true)
			original := bytes.Clone(data)
			p, err := NewProcessor(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			ip, err := imaging.NewProcessor(t.Context(), 100_000_000, 1)
			if err != nil {
				t.Fatal(err)
			}
			info, err := ip.Probe(t.Context(), data)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := p.Extract(t.Context(), data, info)
			if err != nil {
				t.Fatal(err)
			}
			var archive rawArchive
			if err := json.Unmarshal(meta.Raw, &archive); err != nil {
				t.Fatal(err)
			}
			assertTIFFArchivedRange(t, archive, data, tc.paletteOffset, tc.paletteOffset+12)
			if pixels := decodedTIFFPixels(t, data); !bytes.Equal(pixels, tc.pixels) {
				t.Fatalf("source palette pixels=%x, want %x", pixels, tc.pixels)
			}
			for _, mode := range []string{"gps", "all"} {
				t.Run(mode, func(t *testing.T) {
					out, err := p.Scrub(t.Context(), data, "tiff", mode)
					if !errors.Is(err, ErrInvalidMetadata) || out != nil {
						if err == nil {
							t.Fatalf("scrub accepted rewritten palette: source pixels=%x, cloud pixels=%x", tc.pixels, decodedTIFFPixels(t, out))
						}
						t.Fatalf("scrub error=%v, want invalid metadata and no output", err)
					}
				})
			}
			if !bytes.Equal(data, original) {
				t.Fatal("rejected scrub modified the source")
			}
		})
	}
}

func TestTIFFScrubPreservesPaletteAliasingUnchangedDirectoryBytes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		paletteOffset int
		removeArtist  bool
	}{
		{name: "unchanged directory", paletteOffset: 7},
		{name: "unchanged entry in compacted directory", paletteOffset: 10, removeArtist: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := paletteDirectoryAliasTIFF(t, tc.paletteOffset, tc.removeArtist)
			p, err := NewProcessor(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Extract(t.Context(), data, imaging.Info{Format: "tiff"}); err != nil {
				t.Fatal(err)
			}
			out, err := p.Scrub(t.Context(), data, "tiff", "gps")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out[tc.paletteOffset:tc.paletteOffset+12], data[tc.paletteOffset:tc.paletteOffset+12]) {
				t.Fatal("scrub changed a palette alias to unchanged directory bytes")
			}
			if !bytes.Equal(decodedTIFFPixels(t, out), decodedTIFFPixels(t, data)) {
				t.Fatal("scrub changed decoded palette pixels")
			}
			if tc.removeArtist && bytes.Contains(out, []byte("private-artist")) {
				t.Fatal("scrub retained the separate artist payload")
			}
		})
	}
}

func paletteDirectoryAliasTIFF(t *testing.T, paletteOffset int, removeArtist bool) []byte {
	data, payload := onePixelMetadataTIFF(t, 2, 32)
	binary.LittleEndian.PutUint16(data[10+2*12+8:], 1) // One-bit palette index.
	binary.LittleEndian.PutUint16(data[10+4*12+8:], 3) // Palette photometric interpretation.
	artistPos := 10 + 10*12
	artistTag := uint16(0x110) // Safe Model makes the directory unchanged in gps mode.
	if removeArtist {
		artistTag = 0x13b
	}
	binary.LittleEndian.PutUint16(data[artistPos:], artistTag)
	binary.LittleEndian.PutUint16(data[artistPos+2:], 2)
	binary.LittleEndian.PutUint32(data[artistPos+4:], 15)
	binary.LittleEndian.PutUint32(data[artistPos+8:], fixtureOffset(t, payload))
	clear(data[payload : payload+32])
	copy(data[payload:], "private-artist\x00")
	mapPos := 10 + 11*12
	binary.LittleEndian.PutUint16(data[mapPos:], 0x140)
	binary.LittleEndian.PutUint16(data[mapPos+2:], 3)
	binary.LittleEndian.PutUint32(data[mapPos+4:], 6)
	binary.LittleEndian.PutUint32(data[mapPos+8:], fixtureOffset(t, paletteOffset))
	data[len(data)-1] = 0
	return data
}

func decodedTIFFPixels(t *testing.T, data []byte) []byte {
	t.Helper()
	img, err := vips.NewImageFromBuffer(data, &vips.LoadOptions{FailOn: vips.FailOnError})
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	pixels, err := img.WriteToMemory()
	if err != nil {
		t.Fatal(err)
	}
	return pixels
}

func fixtureOffset(t *testing.T, offset int) uint32 {
	t.Helper()
	if offset < 0 || offset > math.MaxUint32 {
		t.Fatal("synthetic TIFF offset exceeds uint32")
		return 0
	}
	if offset > maxSourceBytes {
		t.Fatal("synthetic TIFF offset exceeds the source limit")
		return 0
	}
	return uint32(offset)
}

func assertDecodedTIFFPixel(t *testing.T, data []byte) {
	t.Helper()
	img, err := vips.NewImageFromBuffer(data, &vips.LoadOptions{FailOn: vips.FailOnError})
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	pixels, err := img.WriteToMemory()
	if err != nil {
		t.Fatalf("decode stored TIFF pixels: %v", err)
	}
	if !bytes.Equal(pixels, []byte{0x7b}) {
		t.Fatalf("decoded pixels=%x, want 7b", pixels)
	}
}

func assertTIFFArchivedRange(t *testing.T, archive rawArchive, data []byte, start, end int) {
	t.Helper()
	covered := make([]bool, end-start)
	for _, block := range archive.Blocks {
		if block.Offset < 0 || block.Offset > len(data) || len(block.Data) > len(data)-block.Offset {
			t.Fatal("archive offset is outside the source")
		}
		if !bytes.Equal(block.Data, data[block.Offset:block.Offset+len(block.Data)]) {
			t.Fatal("archive did not preserve exact source bytes")
		}
		for i := max(start, block.Offset); i < min(end, block.Offset+len(block.Data)); i++ {
			covered[i-start] = true
		}
	}
	for i, found := range covered {
		if !found {
			t.Fatalf("metadata source byte at offset %d was not archived", start+i)
		}
	}
}

// Hand-built, uncompressed one-pixel TIFF. The first ten entries define the
// image; remaining unknown metadata entries share a long overlapping buffer.
func onePixelMetadataTIFF(t *testing.T, values, valueSize int) ([]byte, int) {
	t.Helper()
	if values < 1 || values > 4096-10 || valueSize < 5 || valueSize > maxSourceBytes/2 {
		t.Fatal("invalid synthetic TIFF metadata sizes")
	}
	tags := []struct {
		id, kind     uint16
		count, value uint32
	}{
		{0x100, 4, 1, 1}, {0x101, 4, 1, 1}, {0x102, 3, 1, 8},
		{0x103, 3, 1, 1}, {0x106, 3, 1, 1}, {0x111, 4, 1, 0},
		{0x115, 3, 1, 1}, {0x116, 4, 1, 1}, {0x117, 4, 1, 1},
		{0x11c, 3, 1, 1},
	}
	valuesOffset := 8 + 2 + (len(tags)+values)*12 + 4
	for i := range values {
		id := 0x6000 + i
		if id > 0xffff {
			t.Fatal("synthetic TIFF tag exceeds uint16")
		}
		tags = append(tags, struct {
			id, kind     uint16
			count, value uint32
		}{uint16(id), 7, fixtureOffset(t, valueSize), fixtureOffset(t, valuesOffset+i)})
	}
	data := make([]byte, valuesOffset+valueSize+values)
	copy(data, []byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	count := len(tags)
	if count > math.MaxUint16 {
		t.Fatal("synthetic TIFF directory exceeds uint16")
		return nil, 0
	}
	if count > 4096 {
		t.Fatal("synthetic TIFF directory exceeds parser limit")
		return nil, 0
	}
	binary.LittleEndian.PutUint16(data[8:], uint16(count))
	tags[5].value = fixtureOffset(t, len(data)-1)
	for i, tag := range tags {
		pos := 10 + i*12
		binary.LittleEndian.PutUint16(data[pos:], tag.id)
		binary.LittleEndian.PutUint16(data[pos+2:], tag.kind)
		binary.LittleEndian.PutUint32(data[pos+4:], tag.count)
		binary.LittleEndian.PutUint32(data[pos+8:], tag.value)
	}
	for i := valuesOffset; i < len(data)-1; i++ {
		data[i] = "123456789ABCDEFGHIJKLMNOPQRSTUV"[(i-valuesOffset)%31]
	}
	data[len(data)-1] = 0x7b
	return data, valuesOffset
}
