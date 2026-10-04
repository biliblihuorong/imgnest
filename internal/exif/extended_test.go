package exif

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/cshum/vipsgen/vips"
)

func TestTIFFArchiveModesAndNativePixels(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := p.Extract(t.Context(), metadataTIFF(), imaging.Info{Format: "tiff"})
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.CaptureMode != "full-source-fallback" || !bytes.Equal(archive.Blocks[0].Data, metadataTIFF()) {
		t.Fatal("private TIFF fallback was not explicit and exact")
	}
	if _, err := p.Extract(t.Context(), make([]byte, maxSourceBytes+1), imaging.Info{Format: "tiff"}); err == nil {
		t.Fatal("metadata archive source cap ignored")
	}
	imageProc, err := imaging.NewProcessor(t.Context(), 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	source, err := vips.NewImageFromBuffer(jpegMetadata(t, metadataTIFF()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	tiff, err := source.TiffsaveBuffer(&vips.TiffsaveBufferOptions{Keep: vips.KeepNone})
	if err != nil {
		t.Fatal(err)
	}
	meta, err = p.Extract(t.Context(), tiff, imaging.Info{Format: "tiff"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.CaptureMode != "metadata-blocks" {
		t.Fatal("common TIFF unnecessarily retained pixels")
	}
	for _, mode := range []string{"gps", "all"} {
		out, err := p.Scrub(t.Context(), tiff, "tiff", mode)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := imageProc.Probe(t.Context(), out); err != nil {
			t.Fatal(err)
		}
		before, err := vips.NewImageFromBuffer(tiff, nil)
		if err != nil {
			t.Fatal(err)
		}
		after, err := vips.NewImageFromBuffer(out, nil)
		if err != nil {
			before.Close()
			t.Fatal(err)
		}
		a, err := before.WriteToMemory()
		if err != nil {
			t.Fatal(err)
		}
		b, err := after.WriteToMemory()
		if err != nil {
			t.Fatal(err)
		}
		before.Close()
		after.Close()
		if !bytes.Equal(a, b) {
			t.Fatal("TIFF pixels changed")
		}
	}
}

func TestHEIFRawItemsAndGIFXMP(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imaging.NewProcessor(t.Context(), 100_000_000, 1); err != nil {
		t.Fatal(err)
	}
	image, err := vips.NewImageFromBuffer(jpegMetadata(t, metadataTIFF()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	opts := vips.DefaultHeifsaveBufferOptions()
	opts.Q = 80
	opts.Effort = 1
	opts.Bitdepth = 8
	opts.Keep = vips.KeepAll
	for _, format := range []string{"heic", "avif"} {
		if format == "heic" {
			opts.Compression = vips.HeifCompressionHevc
		} else {
			opts.Compression = vips.HeifCompressionAv1
		}
		data, err := image.HeifsaveBuffer(opts)
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := p.Extract(t.Context(), data, imaging.Info{Format: format})
		if err != nil {
			t.Fatal(err)
		}
		var archive rawArchive
		if err := json.Unmarshal(metadata.Raw, &archive); err != nil {
			t.Fatal(err)
		}
		if archive.CaptureMode != "isobmff-items" || len(archive.Blocks) == 0 {
			t.Fatalf("ISOBMFF metadata items missing: source exif=%v, encoded Exif marker=%v, source fields=%v", image.HasField("exif-data"), bytes.Contains(data, []byte("Exif")), image.GetFields())
		}
		if metadata.Model != "Test Model" {
			t.Fatal("HEIF standard camera fields missing")
		}
		for _, block := range archive.Blocks {
			if len(block.Data) == len(data) {
				t.Fatal("HEIF unnecessarily archived its pixels")
			}
		}
	}
	// Minimal valid GIF with one black pixel, plus an XMP application extension.
	gif := []byte{'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0x80, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0x21, 0xff, 11}
	gif = append(gif, []byte("XMP DataXMP")...)
	packet := []byte("private-xmp")
	gif = append(gif, 11)
	gif = append(gif, packet...)
	gif = append(gif, 0, 0x2c, 0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 0x44, 1, 0, 0x3b)
	meta, err := p.Extract(t.Context(), gif, imaging.Info{Format: "gif"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(meta.Raw, []byte("xmp")) {
		t.Fatal("GIF XMP not archived")
	}
	out, err := p.Scrub(t.Context(), gif, "gif", "gps")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, packet) {
		t.Fatal("GIF XMP retained")
	}
	if !bytes.Equal(gif[bytes.IndexByte(gif, 0x2c):], out[bytes.IndexByte(out, 0x2c):]) {
		t.Fatal("GIF compressed pixels changed")
	}
}

func TestMetadataAliasingCannotCorruptImageStructure(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bad := metadataTIFF()
	binary.LittleEndian.PutUint32(bad[8+2+3*12+8:], 0)
	if _, err := p.Scrub(t.Context(), jpegMetadata(t, bad), "jpeg", "gps"); err == nil {
		t.Fatal("sensitive field pointing into TIFF header was accepted")
	}
	if _, err := p.Scrub(t.Context(), pixelAliasTIFF(), "tiff", "gps"); err == nil {
		t.Fatal("sensitive field pointing into pixel strip was accepted")
	}
}

func TestNativeScrubPixelIdentity(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imaging.NewProcessor(t.Context(), 100_000_000, 1); err != nil {
		t.Fatal(err)
	}
	jpegData := jpegMetadata(t, metadataTIFF())
	image, err := vips.NewImageFromBuffer(jpegData, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	save := vips.DefaultWebpsaveBufferOptions()
	save.Keep = vips.KeepAll
	webp, err := image.WebpsaveBuffer(save)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		format string
		data   []byte
	}{{"jpeg", jpegData}, {"png", pngMetadata(t, metadataTIFF())}, {"webp", webp}} {
		for _, mode := range []string{"gps", "all"} {
			t.Run(tc.format+"/"+mode, func(t *testing.T) {
				out, err := p.Scrub(t.Context(), tc.data, tc.format, mode)
				if err != nil {
					t.Fatal(err)
				}
				before, err := vips.NewImageFromBuffer(tc.data, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer before.Close()
				after, err := vips.NewImageFromBuffer(out, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer after.Close()
				a, err := before.WriteToMemory()
				if err != nil {
					t.Fatal(err)
				}
				b, err := after.WriteToMemory()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(a, b) {
					t.Fatal("scrub changed decoded pixels")
				}
				meta, err := p.Extract(t.Context(), out, imaging.Info{Format: tc.format})
				if err != nil {
					t.Fatal(err)
				}
				if meta.GPSLat != nil || meta.GPSLng != nil {
					t.Fatal("scrubbed native original retained GPS")
				}
			})
		}
	}
}

func TestJPEGMetadataBetweenScans(t *testing.T) {
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	base := jpegMetadata(t, metadataTIFF())
	payload := []byte("http://ns.adobe.com/xap/1.0/\x00late-private-xmp")
	segment := []byte{0xff, 0xe1, 0, byte((len(payload) + 2) & 255)}
	segment = append(segment, payload...)
	data := append(append(append([]byte{}, base[:len(base)-2]...), segment...), base[len(base)-2:]...)
	meta, err := p.Extract(t.Context(), data, imaging.Info{Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	var archive rawArchive
	if err := json.Unmarshal(meta.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, block := range archive.Blocks {
		found = found || bytes.Contains(block.Data, []byte("late-private-xmp"))
	}
	if !found {
		t.Fatal("post-SOS metadata was omitted from archive")
	}
	out, err := p.Scrub(t.Context(), data, "jpeg", "gps")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("late-private-xmp")) {
		t.Fatal("post-SOS XMP retained")
	}
}

func TestISOBMFFItemRelativeExtentCannotLeaveIDAT(t *testing.T) {
	infe := []byte{2, 0, 0, 0, 0, 1, 0, 0, 'm', 'i', 'm', 'e', 0}
	infe = append(infe, []byte("application/rdf+xml\x00")...)
	iinf := []byte{0, 0, 0, 0, 0, 1}
	iinf = append(iinf, isoFixtureBox("infe", infe)...)
	iloc := make([]byte, 24)
	iloc[0] = 1
	iloc[4] = 0x44
	binary.BigEndian.PutUint16(iloc[6:], 1)
	binary.BigEndian.PutUint16(iloc[8:], 1)
	binary.BigEndian.PutUint16(iloc[10:], 1)
	binary.BigEndian.PutUint16(iloc[14:], 1)
	binary.BigEndian.PutUint32(iloc[20:], 8)
	meta := []byte{0, 0, 0, 0}
	meta = append(meta, isoFixtureBox("iinf", iinf)...)
	meta = append(meta, isoFixtureBox("iloc", iloc)...)
	meta = append(meta, isoFixtureBox("idat", []byte("<x/>"))...)
	data := isoFixtureBox("ftyp", []byte{'h', 'e', 'i', 'c', 0, 0, 0, 0})
	data = append(data, isoFixtureBox("meta", meta)...)
	data = append(data, isoFixtureBox("mdat", []byte("not metadata"))...)
	if _, err := isobmffBlocks(data); err == nil {
		t.Fatal("item-relative extent escaped its IDAT box")
	}
}

func isoFixtureBox(name string, payload []byte) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out, uint32((uint64(len(payload))+8)&0xffffffff))
	copy(out[4:], name)
	return append(out, payload...)
}

func pixelAliasTIFF() []byte {
	count := 10
	bitsOffset := 8 + 2 + count*12 + 4
	pixelsOffset := bitsOffset + 6
	data := make([]byte, pixelsOffset+7)
	copy(data, []byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	binary.LittleEndian.PutUint16(data[8:], uint16(count))
	tags := [][4]uint32{{0x100, 4, 1, 2}, {0x101, 4, 1, 1}, {0x102, 3, 3, uint32(bitsOffset)}, {0x103, 3, 1, 1}, {0x106, 3, 1, 2}, {0x111, 4, 1, uint32(pixelsOffset)}, {0x115, 3, 1, 3}, {0x116, 4, 1, 1}, {0x117, 4, 1, 6}, {0x13b, 2, 7, uint32(pixelsOffset)}}
	for i, tag := range tags {
		pos := 10 + i*12
		if tag[0] > 65535 || tag[1] > 65535 {
			panic("invalid synthetic tag")
		}
		binary.LittleEndian.PutUint16(data[pos:], uint16(tag[0]&65535))
		binary.LittleEndian.PutUint16(data[pos+2:], uint16(tag[1]&65535))
		binary.LittleEndian.PutUint32(data[pos+4:], tag[2])
		binary.LittleEndian.PutUint32(data[pos+8:], tag[3])
	}
	for i := range 3 {
		binary.LittleEndian.PutUint16(data[bitsOffset+i*2:], 8)
	}
	copy(data[pixelsOffset:], []byte("secret\x00"))
	return data
}
