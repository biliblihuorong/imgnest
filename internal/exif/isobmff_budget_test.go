package exif

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
)

func TestISOBMFFSharedMetadataArchivesSourceOnce(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       string
		payload    []byte
		tiffOffset int
	}{
		{name: "raw TIFF", kind: "exif", payload: minimalTIFF(6)},
		{
			name: "EXIF item offset", kind: "exif", tiffOffset: 4,
			payload: append([]byte{0, 0, 0, 0}, minimalTIFF(6)...),
		},
		{
			name: "EXIF prefix", kind: "exif", tiffOffset: 6,
			payload: append([]byte("Exif\x00\x00"), minimalTIFF(6)...),
		},
		{name: "XMP", kind: "xmp", payload: []byte("<x/>")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := budgetISOFixture(t, tc.payload, []budgetISOItem{
				{id: 9, kind: tc.kind, extents: []rawExtent{{Offset: 0, Length: len(tc.payload)}}},
				{id: 2, kind: tc.kind, extents: []rawExtent{{Offset: 0, Length: len(tc.payload)}}},
			})
			before := bytes.Clone(data)
			blocks, err := isobmffBlocks(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != 1 {
				t.Fatalf("shared source archived %d times, want 1", len(blocks))
			}
			if !bytes.Equal(blocks[0].Data, tc.payload) {
				t.Fatal("shared source payload changed")
			}
			if blocks[0].TIFFOffset != tc.tiffOffset {
				t.Fatalf("TIFF offset = %d, want %d", blocks[0].TIFFOffset, tc.tiffOffset)
			}
			if !bytes.Equal(data, before) {
				t.Fatal("metadata extraction changed the input")
			}
		})
	}
}

func TestISOBMFFSharedMetadataPreservesItemIDsInArchive(t *testing.T) {
	data, _ := budgetISOFixture(t, minimalTIFF(6), []budgetISOItem{
		{id: 9, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 26}}},
		{id: 2, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 26}}},
	})
	p, err := NewProcessor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Extract(t.Context(), data, imaging.Info{Format: "heic"})
	if err != nil {
		t.Fatal(err)
	}
	var archive struct {
		Version     int    `json:"version"`
		CaptureMode string `json:"capture_mode"`
		Blocks      []struct {
			ItemIDs []uint64 `json:"item_ids"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(out.Raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.Version != 1 || archive.CaptureMode != "isobmff-items" {
		t.Fatalf("archive format changed: version=%d mode=%q", archive.Version, archive.CaptureMode)
	}
	if len(archive.Blocks) != 1 || !reflect.DeepEqual(archive.Blocks[0].ItemIDs, []uint64{2, 9}) {
		t.Fatalf("logical item references were lost: %+v", archive.Blocks)
	}
}

func TestISOBMFFArchiveBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		items   []budgetISOItem
		budget  int
		want    []string
		wantIDs [][]uint64
	}{
		{
			name: "independent XMP items share one cumulative budget", payload: []byte("abcdefgh"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}}},
			},
			want: []string{"abcd", "efgh"}, wantIDs: [][]uint64{{1}, {2}},
		},
		{
			name:    "independent EXIF items share one cumulative budget",
			payload: append(minimalTIFF(1), minimalTIFF(1)...), budget: 52,
			items: []budgetISOItem{
				{id: 1, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 26}}},
				{id: 2, kind: "exif", extents: []rawExtent{{Offset: 26, Length: 26}}},
			},
			want: []string{string(minimalTIFF(1)), string(minimalTIFF(1))}, wantIDs: [][]uint64{{1}, {2}},
		},
		{
			name: "shared multiextent EXIF consumes one payload budget", payload: minimalTIFF(1), budget: 26,
			items: []budgetISOItem{
				{id: 9, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 8}, {Offset: 8, Length: 18}}},
				{id: 2, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 8}, {Offset: 8, Length: 18}}},
			},
			want: []string{string(minimalTIFF(1))}, wantIDs: [][]uint64{{2, 9}},
		},
		{
			name: "duplicate XMP item consumes no second budget", payload: []byte("abcd"), budget: 4,
			items: []budgetISOItem{
				{id: 9, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
			},
			want: []string{"abcd"}, wantIDs: [][]uint64{{2, 9}},
		},
		{
			name: "independent identical bytes stay separate", payload: []byte("abcdabcd"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}}},
			},
			want: []string{"abcd", "abcd"}, wantIDs: [][]uint64{{1}, {2}},
		},
		{
			name: "EXIF and XMP on the same source stay separate", payload: minimalTIFF(1), budget: 52,
			items: []budgetISOItem{
				{id: 1, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 26}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 26}}},
			},
			want: []string{string(minimalTIFF(1)), string(minimalTIFF(1))}, wantIDs: [][]uint64{{1}, {2}},
		},
		{
			name: "repeated extent within one item is copied twice", payload: []byte("abcd"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}, {Offset: 0, Length: 4}}},
			},
			want: []string{"abcdabcd"}, wantIDs: [][]uint64{{1}},
		},
		{
			name: "overlapping extents retain logical copies", payload: []byte("abcdef"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}, {Offset: 2, Length: 4}}},
			},
			want: []string{"abcdcdef"}, wantIDs: [][]uint64{{1}},
		},
		{
			name: "partially overlapping items each consume budget", payload: []byte("abcdef"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 2, Length: 4}}},
			},
			want: []string{"abcd", "cdef"}, wantIDs: [][]uint64{{1}, {2}},
		},
		{
			name: "shared ordered multiextent is archived once", payload: []byte("abcdefgh"), budget: 8,
			items: []budgetISOItem{
				{id: 9, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}, {Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}, {Offset: 0, Length: 4}}},
			},
			want: []string{"efghabcd"}, wantIDs: [][]uint64{{2, 9}},
		},
		{
			name: "extent order is part of source identity", payload: []byte("abcdefgh"), budget: 16,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}, {Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}, {Offset: 4, Length: 4}}},
			},
			want: []string{"abcdefgh", "efghabcd"}, wantIDs: [][]uint64{{2}, {1}},
		},
		{
			name: "adjacent split layout remains a distinct source", payload: []byte("abcd"), budget: 8,
			items: []budgetISOItem{
				{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
				{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 2}, {Offset: 2, Length: 2}}},
			},
			want: []string{"abcd", "abcd"}, wantIDs: [][]uint64{{1}, {2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := budgetISOFixture(t, tc.payload, tc.items)
			before := bytes.Clone(data)
			blocks, err := isobmffBlocksWithBudget(data, tc.budget)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != len(tc.want) {
				t.Fatalf("archived %d blocks, want %d", len(blocks), len(tc.want))
			}
			for i, block := range blocks {
				if string(block.Data) != tc.want[i] || !reflect.DeepEqual(block.ItemIDs, tc.wantIDs[i]) {
					t.Fatalf("block %d data=%q item IDs=%v, want data=%q item IDs=%v",
						i, block.Data, block.ItemIDs, tc.want[i], tc.wantIDs[i])
				}
			}
			blocks, err = isobmffBlocksWithBudget(data, tc.budget-1)
			if !errors.Is(err, ErrInvalidMetadata) || blocks != nil {
				t.Fatalf("budget exceeded by one byte: blocks=%d err=%v, want no blocks and ErrInvalidMetadata",
					len(blocks), err)
			}
			if !bytes.Equal(data, before) {
				t.Fatal("budget check changed the source bytes")
			}
		})
	}
}

func TestISOBMFFArchiveBudgetValidation(t *testing.T) {
	data, _ := budgetISOFixture(t, nil, nil)
	for _, tc := range []struct {
		name   string
		budget int
		bad    bool
	}{
		{name: "empty metadata accepts zero budget", budget: 0},
		{name: "negative budget", budget: -1, bad: true},
		{name: "above production budget", budget: maxMetadataBytes + 1, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks, err := isobmffBlocksWithBudget(data, tc.budget)
			if tc.bad {
				if !errors.Is(err, ErrInvalidMetadata) || blocks != nil {
					t.Fatalf("invalid budget returned blocks=%v err=%v", blocks, err)
				}
				return
			}
			if err != nil || len(blocks) != 0 {
				t.Fatalf("empty metadata returned blocks=%v err=%v", blocks, err)
			}
		})
	}
	data, _ = budgetISOFixture(t, []byte("x"), []budgetISOItem{
		{id: 1, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 1}}},
	})
	if blocks, err := isobmffBlocksWithBudget(data, 0); !errors.Is(err, ErrInvalidMetadata) || blocks != nil {
		t.Fatalf("nonempty metadata with zero budget returned blocks=%v err=%v", blocks, err)
	}
}

func TestISOBMFFArchiveBudgetPrecedesPayloadParsing(t *testing.T) {
	// A BigTIFF header would return ErrUnsupportedFormat if payload parsing ran first.
	data, _ := budgetISOFixture(t, []byte{'I', 'I', 43, 0, 0, 0, 0, 0, 'x'}, []budgetISOItem{
		{id: 1, kind: "exif", extents: []rawExtent{{Offset: 0, Length: 8}}},
		{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 8, Length: 1}}},
	})
	if blocks, err := isobmffBlocksWithBudget(data, 8); !errors.Is(err, ErrInvalidMetadata) || blocks != nil {
		t.Fatalf("payload was parsed before cumulative budget rejection: blocks=%v err=%v", blocks, err)
	}
}

func TestISOBMFFArchiveOrderAndExtentsAreDeterministic(t *testing.T) {
	data, base := budgetISOFixture(t, []byte("abcdefgh"), []budgetISOItem{
		{id: 9, kind: "xmp", extents: []rawExtent{{Offset: 4, Length: 4}}},
		{id: 7, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 2}, {Offset: 2, Length: 2}}},
		{id: 2, kind: "xmp", extents: []rawExtent{{Offset: 0, Length: 4}}},
	})
	want := []rawBlock{
		{
			Kind: "xmp", Offset: base, Data: []byte("abcd"), ItemIDs: []uint64{2},
			Extents: []rawExtent{{Offset: base, Length: 4}},
		},
		{
			Kind: "xmp", Offset: base, Data: []byte("abcd"), ItemIDs: []uint64{7},
			Extents: []rawExtent{{Offset: base, Length: 2}, {Offset: base + 2, Length: 2}},
		},
		{
			Kind: "xmp", Offset: base + 4, Data: []byte("efgh"), ItemIDs: []uint64{9},
			Extents: []rawExtent{{Offset: base + 4, Length: 4}},
		},
	}
	for range 32 {
		blocks, err := isobmffBlocks(data)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(blocks, want) {
			t.Fatalf("source order, item ID tie-break or source extents changed: got %+v, want %+v", blocks, want)
		}
	}
}

type budgetISOItem struct {
	id      uint16
	kind    string
	extents []rawExtent // Relative to the fixture's IDAT payload.
}

func budgetISOFixture(t *testing.T, payload []byte, items []budgetISOItem) ([]byte, int) {
	t.Helper()
	itemCount := len(items)
	if itemCount > 3 {
		t.Fatal("fixture exceeds the three-item test bound")
		return nil, 0
	}
	iinf := []byte{0, 0, 0, 0}
	iinf = binary.BigEndian.AppendUint16(iinf, uint16(itemCount))
	iloc := []byte{1, 0, 0, 0, 0x44, 0}
	iloc = binary.BigEndian.AppendUint16(iloc, uint16(itemCount))
	for _, item := range items {
		infe := []byte{2, 0, 0, 0}
		infe = binary.BigEndian.AppendUint16(infe, item.id)
		infe = append(infe, 0, 0)
		switch item.kind {
		case "exif":
			infe = append(infe, []byte("Exif\x00")...)
		case "xmp":
			infe = append(infe, []byte("mime\x00application/rdf+xml\x00")...)
		default:
			t.Fatalf("unsupported fixture kind %q", item.kind)
		}
		iinf = append(iinf, isoFixtureBox("infe", infe)...)
		iloc = binary.BigEndian.AppendUint16(iloc, item.id)
		iloc = append(iloc, 0, 1, 0, 0) // IDAT-relative construction, local data reference.
		extentCount := len(item.extents)
		if extentCount > 4 {
			t.Fatal("fixture exceeds the four-extent test bound")
			return nil, 0
		}
		iloc = binary.BigEndian.AppendUint16(iloc, uint16(extentCount))
		for _, extent := range item.extents {
			if extent.Offset < 0 || extent.Offset > 128<<10 || extent.Offset > len(payload) {
				t.Fatal("fixture offset is outside its IDAT payload")
				return nil, 0
			}
			if extent.Length < 1 || extent.Length > 128<<10 || extent.Length > len(payload)-extent.Offset {
				t.Fatal("fixture extent is outside its IDAT payload")
				return nil, 0
			}
			iloc = binary.BigEndian.AppendUint32(iloc, uint32(extent.Offset))
			iloc = binary.BigEndian.AppendUint32(iloc, uint32(extent.Length))
		}
	}
	meta := []byte{0, 0, 0, 0}
	meta = append(meta, isoFixtureBox("iinf", iinf)...)
	meta = append(meta, isoFixtureBox("iloc", iloc)...)
	data := isoFixtureBox("ftyp", []byte{'h', 'e', 'i', 'c', 0, 0, 0, 0})
	idata := len(data) + 8 + len(meta) + 8
	meta = append(meta, isoFixtureBox("idat", payload)...)
	data = append(data, isoFixtureBox("meta", meta)...)
	if len(data) > 128<<10 {
		t.Fatal("fixture exceeds the 128 KiB source test bound")
	}
	return data, idata
}
