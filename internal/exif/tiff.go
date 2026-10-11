package exif

import (
	"bytes"
	"encoding/binary"
)

type tiffEntry struct {
	id, kind          uint16
	count             uint32
	pos, offset, size int
	role              string
}
type tiffDirectory struct {
	offset  int
	next    uint32
	entries []tiffEntry
	role    string
}
type tiffDocument struct {
	data                            []byte
	order                           binary.ByteOrder
	dirs                            []tiffDirectory
	orientation                     int
	gpsLat, gpsLng, gpsAlt, private bool
	pixels                          []byteRange
}
type byteRange struct{ start, end int }

func parseTIFF(data []byte) (*tiffDocument, error) {
	if len(data) < 8 {
		return nil, ErrInvalidMetadata
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, ErrInvalidMetadata
	}
	if order.Uint16(data[2:4]) != 42 {
		return nil, ErrUnsupportedFormat
	}
	doc := &tiffDocument{data: data, order: order, dirs: []tiffDirectory{}}
	visited := map[uint32]bool{}
	var walk func(uint32, string) error
	walk = func(offset uint32, role string) error {
		if offset == 0 {
			return nil
		}
		if visited[offset] || len(visited) >= 64 {
			return ErrInvalidMetadata
		}
		visited[offset] = true
		if uint64(offset)+2 > uint64(len(data)) {
			return ErrInvalidMetadata
		}
		start := int(offset)
		count := int(order.Uint16(data[start:]))
		if count > 4096 || count > (len(data)-start-6)/12 {
			return ErrInvalidMetadata
		}
		end := start + 2 + count*12 + 4
		if end > len(data) {
			return ErrInvalidMetadata
		}
		dir := tiffDirectory{offset: start, next: order.Uint32(data[end-4:]), role: role, entries: []tiffEntry{}}
		children := []struct {
			offset uint32
			role   string
		}{}
		for i := range count {
			pos := start + 2 + i*12
			entry := tiffEntry{id: order.Uint16(data[pos:]), kind: order.Uint16(data[pos+2:]), count: order.Uint32(data[pos+4:]), pos: pos, role: role}
			width := typeWidth(entry.kind)
			if width <= 0 || width > 8 {
				return ErrInvalidMetadata
			}
			size := uint64(entry.count) * uint64(width)
			if size > uint64(len(data)) {
				return ErrInvalidMetadata
			}
			bounded, err := boundedOffset(size)
			if err != nil {
				return err
			}
			entry.size = bounded
			entry.offset = pos + 8
			if size > 4 {
				entry.offset = int(order.Uint32(data[pos+8:]))
			}
			if entry.offset < 0 || entry.offset > len(data) || entry.size > len(data)-entry.offset {
				return ErrInvalidMetadata
			}
			if entry.id == 0x112 && entry.kind == 3 && entry.count == 1 {
				doc.orientation = int(order.Uint16(data[entry.offset:]))
				if doc.orientation < 1 || doc.orientation > 8 {
					return ErrInvalidMetadata
				}
			}
			if role == "gps" {
				switch entry.id {
				case 2:
					doc.gpsLat = true
				case 4:
					doc.gpsLng = true
				case 6:
					doc.gpsAlt = true
				}
			}
			childRole := ""
			switch entry.id {
			case 0x8769:
				childRole = "exif"
			case 0x8825:
				childRole = "gps"
			case 0xa005:
				childRole = "interop"
			case 0x14a:
				childRole = "image"
			}
			if childRole != "" {
				if entry.kind != 4 && entry.kind != 13 {
					return ErrInvalidMetadata
				}
				for j := range int(entry.count) {
					children = append(children, struct {
						offset uint32
						role   string
					}{offset: order.Uint32(data[entry.offset+j*4:]), role: childRole})
				}
			}
			if entry.id == 0x927c || entry.id == 0xc634 {
				doc.private = true
			}
			if entry.id >= 0xc000 && entry.kind == 4 && entry.count > 0 {
				doc.private = true
			}
			dir.entries = append(dir.entries, entry)
		}
		doc.dirs = append(doc.dirs, dir)
		for _, child := range children {
			if err := walk(child.offset, child.role); err != nil {
				return err
			}
		}
		return walk(dir.next, role)
	}
	if err := walk(order.Uint32(data[4:]), "image"); err != nil {
		return nil, err
	}
	for _, dir := range doc.dirs {
		entries := map[uint16]tiffEntry{}
		for _, entry := range dir.entries {
			entries[entry.id] = entry
		}
		for _, pair := range [][2]uint16{{0x111, 0x117}, {0x144, 0x145}, {0x201, 0x202}} {
			offsets, hasOffsets := entries[pair[0]]
			lengths, hasLengths := entries[pair[1]]
			if hasOffsets != hasLengths {
				return nil, ErrInvalidMetadata
			}
			if !hasOffsets {
				continue
			}
			if offsets.count != lengths.count {
				return nil, ErrInvalidMetadata
			}
			for i := range int(offsets.count) {
				offset, err := entryUint(data, order, offsets, i)
				if err != nil {
					return nil, err
				}
				length, err := entryUint(data, order, lengths, i)
				if err != nil {
					return nil, err
				}
				if offset > uint64(len(data)) || length > uint64(len(data))-offset {
					return nil, ErrInvalidMetadata
				}
				start, err := boundedOffset(offset)
				if err != nil {
					return nil, err
				}
				end, err := boundedOffset(offset + length)
				if err != nil {
					return nil, err
				}
				doc.pixels = append(doc.pixels, byteRange{start: start, end: end})
			}
		}
	}
	for _, dir := range doc.dirs {
		end := dir.offset + 2 + len(dir.entries)*12 + 4
		for _, pixels := range doc.pixels {
			if overlaps(dir.offset, end, pixels.start, pixels.end) {
				return nil, ErrInvalidMetadata
			}
		}
		for _, entry := range dir.entries {
			if entry.size <= 4 || structuralTag(entry.id) {
				continue
			}
			for _, pixels := range doc.pixels {
				if overlaps(entry.offset, entry.offset+entry.size, pixels.start, pixels.end) {
					return nil, ErrInvalidMetadata
				}
			}
		}
	}
	return doc, nil
}

func entryUint(data []byte, order binary.ByteOrder, entry tiffEntry, index int) (uint64, error) {
	pos := entry.offset + index*typeWidth(entry.kind)
	switch entry.kind {
	case 3:
		return uint64(order.Uint16(data[pos:])), nil
	case 4, 13:
		return uint64(order.Uint32(data[pos:])), nil
	case 16, 18:
		return order.Uint64(data[pos:]), nil
	}
	return 0, ErrInvalidMetadata
}

func typeWidth(kind uint16) int {
	switch kind {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11, 13:
		return 4
	case 5, 10, 12, 16, 17, 18:
		return 8
	}
	return 0
}

func (d *tiffDocument) archive() []rawBlock {
	blocks := []rawBlock{{Kind: "tiff-header", Offset: 0, Data: bytes.Clone(d.data[:8])}}
	seen := map[int]bool{}
	for _, dir := range d.dirs {
		size := 2 + len(dir.entries)*12 + 4
		blocks = append(blocks, rawBlock{Kind: "tiff-ifd", Offset: dir.offset, Data: bytes.Clone(d.data[dir.offset : dir.offset+size])})
		for _, entry := range dir.entries {
			if entry.size > 4 && !seen[entry.offset] {
				seen[entry.offset] = true
				blocks = append(blocks, rawBlock{Kind: "tiff-value", Offset: entry.offset, Data: bytes.Clone(d.data[entry.offset : entry.offset+entry.size])})
			}
		}
	}
	return blocks
}

func (d *tiffDocument) scrub(mode string, wholeTIFF bool) ([]byte, error) {
	out := bytes.Clone(d.data)
	for _, dir := range d.dirs {
		kept := []tiffEntry{}
		for _, entry := range dir.entries {
			// Embedded EXIF may carry vendor-private tags (IDs at or above
			// 0xC000) that hold device or owner identifiers beyond the named
			// list; the gps profile strips them too. Whole-TIFF documents
			// never reach this branch with such tags: parseTIFF flags them
			// and the upload is refused instead.
			remove := dir.role == "gps" || entry.id == 0x8825 || entry.id == 0x13b || entry.id == 0xa430 || entry.id == 0xa431 || entry.id == 0xa435 || entry.id == 0x927c || entry.id == 0xc634 || entry.id == 700 || (!wholeTIFF && entry.id >= 0xc000)
			if mode == "all" {
				remove = entry.id != 0x112 && entry.id != 0x8773
				if wholeTIFF && structuralTag(entry.id) {
					remove = false
				}
			}
			if remove {
				if entry.size > 4 && overlaps(entry.offset, entry.offset+entry.size, 0, 8) {
					return nil, ErrInvalidMetadata
				}
				for _, pixels := range d.pixels {
					if overlaps(entry.offset, entry.offset+entry.size, pixels.start, pixels.end) {
						return nil, ErrInvalidMetadata
					}
				}
				if entry.size > 4 {
					for _, other := range d.dirs {
						end := other.offset + 2 + len(other.entries)*12 + 4
						if overlaps(entry.offset, entry.offset+entry.size, other.offset, end) {
							return nil, ErrInvalidMetadata
						}
					}
				}
				clear(out[entry.offset : entry.offset+entry.size])
				continue
			}
			kept = append(kept, entry)
		}
		start := dir.offset + 2
		end := start + len(dir.entries)*12 + 4
		copyEntries := make([]byte, len(kept)*12)
		for i, entry := range kept {
			copy(copyEntries[i*12:], d.data[entry.pos:entry.pos+12])
		}
		clear(out[start:end])
		copy(out[start:], copyEntries)
		if len(kept) > 4096 {
			return nil, ErrInvalidMetadata
		}
		d.order.PutUint16(out[dir.offset:], uint16(len(kept)&65535))
		d.order.PutUint32(out[start+len(kept)*12:], dir.next)
	}
	if _, err := parseTIFF(out); err != nil {
		return nil, err
	}
	return out, nil
}

func structuralTag(id uint16) bool {
	switch id {
	case 0xfe, 0xff, 0x100, 0x101, 0x102, 0x103, 0x106, 0x111, 0x115, 0x116, 0x117, 0x11a, 0x11b, 0x11c, 0x128, 0x13d, 0x140, 0x142, 0x143, 0x144, 0x145, 0x14a, 0x152, 0x153, 0x15b, 0x201, 0x202, 0x211, 0x212, 0x213, 0x214:
		return true
	}
	return false
}
func overlaps(a, b, c, d int) bool { return a < d && c < b }
func minimalTIFF(orientation int) []byte {
	if orientation == 0 {
		orientation = 1
	}
	return []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(orientation), 0, 0, 0, 0, 0, 0, 0}
}
