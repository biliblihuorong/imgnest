package exif

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/jpeg"
	"slices"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/cshum/vipsgen/vips"
)

// Shared source extents must be archived once without collapsing equal bytes at
// distinct source positions. These tiny fixtures also prove native reachability.
func TestISOBMFFNativeSharedMetadataArchive(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	ip, err := imaging.NewProcessor(ctx, 1_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := NewProcessor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, 8, 6)), nil); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("Exif\x00\x00"), budgetNativeTIFF(t)...)
	baseJPEG := append([]byte{}, jpegBytes.Bytes()[:2]...)
	baseJPEG = append(baseJPEG, 0xff, 0xe1, byte(((len(payload)+2)>>8)&255), byte((len(payload)+2)&255))
	baseJPEG = append(baseJPEG, payload...)
	baseJPEG = append(baseJPEG, jpegBytes.Bytes()[2:]...)
	img, err := vips.NewImageFromBuffer(baseJPEG, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	for _, format := range []string{"avif", "heic"} {
		t.Run(format, func(t *testing.T) {
			opts := vips.DefaultHeifsaveBufferOptions()
			opts.Q, opts.Effort, opts.Bitdepth, opts.Keep = 80, 1, 8, vips.KeepAll
			opts.Compression = vips.HeifCompressionAv1
			if format == "heic" {
				opts.Compression = vips.HeifCompressionHevc
			}
			base, err := img.HeifsaveBuffer(opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"native", "separate", "shared"} {
				t.Run(mode, func(t *testing.T) {
					data := bytes.Clone(base)
					if mode != "native" {
						data = budgetNativeDuplicateMetadata(t, base, mode == "shared")
					}
					budgetNativeAssertArchive(ctx, t, budgetNativeCase{
						data: data, format: format, mode: mode, imaging: ip, metadata: ep,
					})
				})
			}
		})
	}
}

type budgetNativeCase struct {
	data     []byte
	format   string
	mode     string
	imaging  *imaging.VipsProcessor
	metadata *Processor
}

type budgetNativeExtent struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

func budgetNativeAssertArchive(ctx context.Context, t *testing.T, tc budgetNativeCase) {
	t.Helper()
	if len(tc.data) > 128<<10 {
		t.Fatal("native fixture exceeds 128 KiB source bound")
	}
	before := bytes.Clone(tc.data)
	items := budgetNativeMetadataItems(t, tc.data)
	wantItems, wantBlocks := 2, 2
	if tc.mode == "native" {
		wantItems, wantBlocks = 1, 1
	}
	if tc.mode == "shared" {
		wantBlocks = 1
	}
	if len(items) != wantItems {
		t.Fatalf("metadata items=%d, want %d", len(items), wantItems)
	}
	ids := make([]uint64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	first := items[ids[0]]
	wantPayload := tc.data[first.Offset : first.Offset+first.Length]
	if len(ids) == 2 {
		second := items[ids[1]]
		if !bytes.Equal(wantPayload, tc.data[second.Offset:second.Offset+second.Length]) {
			t.Fatal("native controls must contain identical metadata payload bytes")
		}
		if (first == second) != (tc.mode == "shared") {
			t.Fatal("native control source extents do not match the requested alias layout")
		}
	}
	info, err := tc.imaging.Probe(ctx, tc.data)
	if err != nil {
		t.Fatalf("Probe rejected tiny native %s fixture: %v", tc.mode, err)
	}
	if info.Format != tc.format {
		t.Fatalf("Probe format=%q, want %q", info.Format, tc.format)
	}
	metadata, err := tc.metadata.Extract(ctx, tc.data, info)
	if err != nil {
		t.Fatalf("Extract rejected tiny native %s fixture: %v", tc.mode, err)
	}
	if !bytes.Equal(before, tc.data) {
		t.Fatal("Probe/Extract changed the source input")
	}
	if metadata.Make != "Canon" || metadata.Model != "Budget Native" || metadata.Orientation != 1 {
		t.Fatal("native archive did not preserve Make, Model and Orientation")
	}
	var archive struct {
		Version     int    `json:"version"`
		Format      string `json:"format"`
		CaptureMode string `json:"capture_mode"`
		Blocks      []struct {
			Kind       string               `json:"kind"`
			Offset     int                  `json:"offset"`
			Order      int                  `json:"order"`
			Data       []byte               `json:"data"`
			TIFFOffset int                  `json:"tiff_offset"`
			Extents    []budgetNativeExtent `json:"extents"`
			ItemIDs    []uint64             `json:"item_ids"`
		} `json:"blocks"`
	}
	if len(metadata.Raw) > 256<<10 {
		t.Fatal("native raw archive exceeds 256 KiB bound")
	}
	if err := json.Unmarshal(metadata.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.Version != 1 || archive.Format != tc.format || archive.CaptureMode != "isobmff-items" {
		t.Fatal("native archive version, format or capture mode changed")
	}
	if len(archive.Blocks) != wantBlocks {
		t.Fatalf("archive blocks=%d, want %d (shared source extents must be archived once)", len(archive.Blocks), wantBlocks)
	}
	archivedBytes := 0
	for i, block := range archive.Blocks {
		archivedBytes += len(block.Data)
		wantIDs := ids
		if tc.mode != "shared" {
			wantIDs = ids[i : i+1]
		}
		if !slices.Equal(block.ItemIDs, wantIDs) {
			t.Errorf("block %d item_ids=%v, want %v", i, block.ItemIDs, wantIDs)
		}
		wantExtent := items[wantIDs[0]]
		if block.Kind != "exif" || block.Order != i || block.Offset != wantExtent.Offset {
			t.Fatal("native block kind, order or source offset changed")
		}
		if !slices.Equal(block.Extents, []budgetNativeExtent{wantExtent}) {
			t.Fatal("native archive extents differ from the independent source item locations")
		}
		if !bytes.Equal(block.Data, wantPayload) {
			t.Fatal("native archive payload differs from the original source extent")
		}
		if block.TIFFOffset < 4 || block.TIFFOffset+8 > len(block.Data) {
			t.Fatal("native archive TIFF offset is outside the original item payload")
		}
	}
	if archivedBytes > 256<<10 || archivedBytes != wantBlocks*len(wantPayload) {
		t.Fatal("native archive payload was lost or copied more than once per unique source")
	}
}

// Only standard non-sensitive fields are needed for native codec round-trips.
func budgetNativeTIFF(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 50)
	copy(data, []byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	binary.LittleEndian.PutUint16(data[8:], 3)
	for i, field := range []struct {
		id   uint16
		text string
	}{{id: 0x10f, text: "Canon\x00"}, {id: 0x110, text: "Budget Native\x00"}} {
		textSize := len(field.text)
		if textSize > 256 {
			t.Fatal("native fixture camera text exceeds its small field bound")
			return nil
		}
		dataOffset := len(data)
		if dataOffset > 128<<10 {
			t.Fatal("native fixture TIFF offset exceeds the source bound")
			return nil
		}
		entry := 10 + i*12
		binary.LittleEndian.PutUint16(data[entry:], field.id)
		binary.LittleEndian.PutUint16(data[entry+2:], 2)
		binary.LittleEndian.PutUint32(data[entry+4:], uint32(textSize))
		binary.LittleEndian.PutUint32(data[entry+8:], uint32(dataOffset))
		data = append(data, field.text...)
	}
	binary.LittleEndian.PutUint16(data[34:], 0x112)
	binary.LittleEndian.PutUint16(data[36:], 3)
	binary.LittleEndian.PutUint32(data[38:], 1)
	binary.LittleEndian.PutUint16(data[42:], 1)
	return data
}

type budgetNativeBox struct {
	kind       string
	start, end int
	payload    []byte
}

func budgetNativeBoxes(t *testing.T, data []byte, start, end int) []budgetNativeBox {
	t.Helper()
	boxes := []budgetNativeBox{}
	for pos := start; pos < end; {
		size, header, err := boxSize(data[:end], pos)
		if err != nil {
			t.Fatal(err)
		}
		boxes = append(boxes, budgetNativeBox{
			kind: string(data[pos+4 : pos+8]), start: pos, end: pos + size,
			payload: bytes.Clone(data[pos+header : pos+size]),
		})
		pos += size
	}
	return boxes
}

func budgetNativeBoxBytes(t *testing.T, kind string, payload []byte) []byte {
	t.Helper()
	size := len(payload) + 8
	if size < 8 || size > 128<<10 {
		t.Fatal("native fixture box exceeds the 128 KiB source bound")
		return nil
	}
	b := make([]byte, 8, len(payload)+8)
	binary.BigEndian.PutUint32(b, uint32(size))
	copy(b[4:], kind)
	return append(b, payload...)
}

// Read fixture IDs and item locations separately from production item parsers.
// The fixture uses native iinf0/infe2 and mdat construction with one extent.
func budgetNativeMetadataItems(t *testing.T, data []byte) map[uint64]budgetNativeExtent {
	t.Helper()
	metadataIDs := map[uint64]bool{}
	locations := map[uint64]budgetNativeExtent{}
	for _, top := range budgetNativeBoxes(t, data, 0, len(data)) {
		if top.kind != "meta" {
			continue
		}
		for _, child := range budgetNativeBoxes(t, data, top.start+12, top.end) {
			switch child.kind {
			case "iinf":
				if len(child.payload) < 6 || child.payload[0] != 0 {
					t.Fatal("native fixture requires iinf v0")
				}
				infos := budgetNativeBoxes(t, data, child.start+14, child.end)
				if len(infos) != int(binary.BigEndian.Uint16(child.payload[4:6])) {
					t.Fatal("native fixture iinf count mismatch")
				}
				for _, info := range infos {
					if info.kind != "infe" || len(info.payload) < 12 || info.payload[0] != 2 {
						t.Fatal("native fixture requires infe v2")
					}
					if string(info.payload[8:12]) == "Exif" {
						metadataIDs[uint64(binary.BigEndian.Uint16(info.payload[4:6]))] = true
					}
				}
			case "iloc":
				p := child.payload
				if len(p) < 8 || p[0] != 0 {
					t.Fatal("native fixture requires iloc v0")
				}
				offsetSize, lengthSize, baseSize := int(p[4]>>4), int(p[4]&15), int(p[5]>>4)
				pos := 6
				count := budgetNativeUint(t, p, &pos, 2)
				for range count {
					id := budgetNativeUint(t, p, &pos, 2)
					if budgetNativeUint(t, p, &pos, 2) != 0 {
						t.Fatal("native fixture requires local item data")
					}
					base := budgetNativeUint(t, p, &pos, baseSize)
					if budgetNativeUint(t, p, &pos, 2) != 1 {
						t.Fatal("native fixture requires exactly one source extent per item")
					}
					offset := base + budgetNativeUint(t, p, &pos, offsetSize)
					length := budgetNativeUint(t, p, &pos, lengthSize)
					if offset > uint64(len(data)) || length == 0 || length > uint64(len(data))-offset {
						t.Fatal("native fixture extent is outside the source")
					}
					if offset > 128<<10 || length > 128<<10 {
						t.Fatal("native fixture extent exceeds the 128 KiB source bound")
						return nil
					}
					locations[id] = budgetNativeExtent{Offset: int(offset), Length: int(length)}
				}
				if pos != len(p) {
					t.Fatal("native fixture iloc has trailing bytes")
				}
			}
		}
	}
	items := map[uint64]budgetNativeExtent{}
	for id := range metadataIDs {
		extent, ok := locations[id]
		if !ok {
			t.Fatal("native fixture metadata location missing")
		}
		items[id] = extent
	}
	if len(items) == 0 || len(items) > 2 {
		t.Fatal("native fixture must contain one or two metadata items")
	}
	return items
}

func budgetNativeUint(t *testing.T, data []byte, pos *int, size int) uint64 {
	t.Helper()
	if size < 0 || size > 8 || *pos > len(data) || size > len(data)-*pos {
		t.Fatal("native fixture integer field is outside its box")
	}
	var value uint64
	for _, b := range data[*pos : *pos+size] {
		value = value<<8 | uint64(b)
	}
	*pos += size
	return value
}

func budgetNativeDuplicateMetadata(t *testing.T, base []byte, shared bool) []byte {
	t.Helper()
	tops := budgetNativeBoxes(t, base, 0, len(base))
	var meta budgetNativeBox
	for _, b := range tops {
		if b.kind == "meta" {
			meta = b
		}
	}
	if meta.kind == "" {
		t.Fatal("native fixture has no meta")
	}
	children := budgetNativeBoxes(t, base, meta.start+12, meta.end)
	items := map[uint64]string{}
	locations := map[uint64]isoItem{}
	infos := []budgetNativeBox{}
	iinfHeader := []byte{}
	for _, b := range children {
		switch b.kind {
		case "iinf":
			if err := readItemInfo(b.payload, items); err != nil {
				t.Fatal(err)
			}
			if b.payload[0] != 0 {
				t.Fatal("native fixture requires iinf v0")
			}
			iinfHeader = bytes.Clone(b.payload[:6])
			infos = budgetNativeBoxes(t, base, b.start+14, b.end)
		case "iloc":
			if err := readItemLocations(b.payload, len(base), -1, -1, locations); err != nil {
				t.Fatal(err)
			}
		case "idat":
			t.Fatal("native fixture requires mdat construction")
		}
	}
	var exifID, maxID uint64
	metadataCount := 0
	for id, kind := range items {
		maxID = max(maxID, id)
		if kind == "exif" {
			exifID = id
		}
		if kind == "exif" || kind == "xmp" {
			metadataCount++
		}
	}
	if exifID == 0 || maxID >= 65535 || metadataCount != 1 {
		t.Fatal("native fixture requires one EXIF item and an available duplicate ID")
	}
	newID := maxID + 1
	duplicateInfo := []byte{}
	for _, b := range infos {
		if len(b.payload) < 12 || b.payload[0] != 2 {
			t.Fatal("native fixture requires infe v2")
		}
		if uint64(binary.BigEndian.Uint16(b.payload[4:6])) == exifID {
			duplicateInfo = bytes.Clone(b.payload)
		}
	}
	if len(duplicateInfo) == 0 {
		t.Fatal("native fixture EXIF info missing")
	}
	infoCount := len(infos) + 1
	if infoCount > 4096 {
		t.Fatal("native fixture item info count exceeds the parser bound")
		return nil
	}
	binary.BigEndian.PutUint16(duplicateInfo[4:6], uint16(newID))
	binary.BigEndian.PutUint16(iinfHeader[4:6], uint16(infoCount))
	newIinf := iinfHeader
	for _, b := range infos {
		newIinf = append(newIinf, budgetNativeBoxBytes(t, b.kind, b.payload)...)
	}
	newIinf = append(newIinf, budgetNativeBoxBytes(t, "infe", duplicateInfo)...)
	newIinfBox := budgetNativeBoxBytes(t, "iinf", newIinf)
	old := locations[exifID]
	if len(old.extents) != 1 {
		t.Fatal("native fixture requires one metadata extent")
	}
	metadataPayload := bytes.Clone(base[old.extents[0].Offset : old.extents[0].Offset+old.extents[0].Length])
	locations[newID] = isoItem{id: newID, extents: append([]rawExtent{}, old.extents...)}
	ids := make([]uint64, 0, len(locations))
	for id := range locations {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	// Retain the native item graph by duplicating its metadata association too.
	newIrefBox := []byte{}
	for _, b := range children {
		if b.kind != "iref" {
			continue
		}
		if len(b.payload) < 4 || b.payload[0] != 0 {
			t.Fatal("native fixture requires iref v0")
		}
		refs := budgetNativeBoxes(t, base, b.start+12, b.end)
		refPayload := bytes.Clone(b.payload[:4])
		duplicate := []byte{}
		for _, ref := range refs {
			refPayload = append(refPayload, budgetNativeBoxBytes(t, ref.kind, ref.payload)...)
			if ref.kind == "cdsc" && len(ref.payload) >= 2 && uint64(binary.BigEndian.Uint16(ref.payload[:2])) == exifID {
				duplicate = bytes.Clone(ref.payload)
				binary.BigEndian.PutUint16(duplicate[:2], uint16(newID))
			}
		}
		if len(duplicate) == 0 {
			t.Fatal("native fixture metadata has no cdsc association")
		}
		refPayload = append(refPayload, budgetNativeBoxBytes(t, "cdsc", duplicate)...)
		newIrefBox = budgetNativeBoxBytes(t, "iref", refPayload)
	}
	if len(newIrefBox) == 0 {
		t.Fatal("native fixture metadata graph missing iref")
	}
	makeIloc := func(delta int, finalLength int) []byte {
		p := []byte{0, 0, 0, 0, 0x44, 0, 0, 0}
		locationCount := len(ids)
		if locationCount > 4096 {
			t.Fatal("native fixture item location count exceeds the parser bound")
			return nil
		}
		binary.BigEndian.PutUint16(p[6:8], uint16(locationCount))
		for _, id := range ids {
			if id > 65535 {
				t.Fatal("native fixture item ID exceeds infe v2 bound")
				return nil
			}
			item := locations[id]
			extentCount := len(item.extents)
			if extentCount > 4096 {
				t.Fatal("native fixture extent count exceeds the parser bound")
				return nil
			}
			entry := make([]byte, 6)
			binary.BigEndian.PutUint16(entry[:2], uint16(id))
			binary.BigEndian.PutUint16(entry[4:6], uint16(extentCount))
			p = append(p, entry...)
			for _, extent := range item.extents {
				offset := extent.Offset
				if offset < meta.end {
					t.Fatal("native fixture extent precedes meta end")
				}
				offset += delta
				if id == newID && !shared {
					offset = finalLength + 8
				}
				length := extent.Length
				if offset < 0 || offset > 128<<10 {
					t.Fatal("native fixture output offset exceeds the 128 KiB source bound")
					return nil
				}
				if length <= 0 || length > 128<<10 {
					t.Fatal("native fixture output length exceeds the 128 KiB source bound")
					return nil
				}
				extentBytes := make([]byte, 8)
				binary.BigEndian.PutUint32(extentBytes[:4], uint32(offset))
				binary.BigEndian.PutUint32(extentBytes[4:], uint32(length))
				p = append(p, extentBytes...)
			}
		}
		return budgetNativeBoxBytes(t, "iloc", p)
	}
	makeMeta := func(iloc []byte) []byte {
		p := bytes.Clone(base[meta.start+8 : meta.start+12])
		for _, b := range children {
			switch b.kind {
			case "iinf":
				p = append(p, newIinfBox...)
			case "iloc":
				p = append(p, iloc...)
			case "iref":
				p = append(p, newIrefBox...)
			default:
				p = append(p, budgetNativeBoxBytes(t, b.kind, b.payload)...)
			}
		}
		return budgetNativeBoxBytes(t, "meta", p)
	}
	placeholder := makeMeta(makeIloc(0, len(base)))
	delta := len(placeholder) - (meta.end - meta.start)
	newMeta := makeMeta(makeIloc(delta, len(base)+delta))
	out := []byte{}
	for _, b := range tops {
		if b.kind == "meta" {
			out = append(out, newMeta...)
		} else {
			out = append(out, budgetNativeBoxBytes(t, b.kind, b.payload)...)
		}
	}
	// Both controls retain the same source size and bytes; only iloc differs.
	return append(out, budgetNativeBoxBytes(t, "mdat", metadataPayload)...)
}
