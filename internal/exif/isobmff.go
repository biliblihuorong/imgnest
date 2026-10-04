package exif

import (
	"bytes"
	"encoding/binary"
)

type isoItem struct {
	id      uint64
	kind    string
	extents []rawExtent
}

func isobmffBlocks(data []byte) ([]rawBlock, error) {
	items := map[uint64]string{}
	locations := map[uint64]isoItem{}
	idatastart := -1
	idataend := -1
	iloc := []byte{}
	for pos := 0; pos < len(data); {
		size, header, err := boxSize(data, pos)
		if err != nil {
			return nil, err
		}
		if string(data[pos+4:pos+8]) == "meta" {
			start := pos + header + 4
			if start > pos+size {
				return nil, ErrInvalidMetadata
			}
			for child := start; child < pos+size; {
				childSize, childHeader, err := boxSize(data[:pos+size], child)
				if err != nil {
					return nil, err
				}
				payload := data[child+childHeader : child+childSize]
				switch string(data[child+4 : child+8]) {
				case "iinf":
					if err := readItemInfo(payload, items); err != nil {
						return nil, err
					}
				case "iloc":
					iloc = payload
				case "idat":
					idatastart = child + childHeader
					idataend = child + childSize
				}
				child += childSize
			}
		}
		pos += size
	}
	if len(iloc) > 0 {
		if err := readItemLocations(iloc, len(data), idatastart, idataend, locations); err != nil {
			return nil, err
		}
	}
	blocks := []rawBlock{}
	for id, kind := range items {
		if kind != "exif" && kind != "xmp" {
			continue
		}
		item, ok := locations[id]
		if !ok || len(item.extents) == 0 {
			return nil, ErrInvalidMetadata
		}
		payload := []byte{}
		for _, extent := range item.extents {
			if extent.Length > maxSourceBytes-len(payload) {
				return nil, ErrInvalidMetadata
			}
			payload = append(payload, data[extent.Offset:extent.Offset+extent.Length]...)
		}
		block := rawBlock{Kind: kind, Offset: item.extents[0].Offset, Data: payload, Extents: item.extents}
		if kind == "exif" {
			switch {
			case bytes.HasPrefix(payload, []byte("Exif\x00\x00")):
				block.TIFFOffset = 6
			case len(payload) >= 4 && string(payload[:2]) != "II" && string(payload[:2]) != "MM":
				offset := uint64(binary.BigEndian.Uint32(payload[:4])) + 4
				if offset > uint64(len(payload)) {
					return nil, ErrInvalidMetadata
				}
				bounded, offsetErr := boundedOffset(offset)
				if offsetErr != nil {
					return nil, offsetErr
				}
				block.TIFFOffset = bounded
			}
			if _, err := parseTIFF(payload[block.TIFFOffset:]); err != nil {
				return nil, err
			}
		}
		blocks = append(blocks, block)
	}
	// Source order is retained rather than Go map iteration order.
	for i := 1; i < len(blocks); i++ {
		for j := i; j > 0 && blocks[j].Offset < blocks[j-1].Offset; j-- {
			blocks[j], blocks[j-1] = blocks[j-1], blocks[j]
		}
	}
	return blocks, nil
}

func readItemInfo(data []byte, items map[uint64]string) error {
	if len(data) < 6 {
		return ErrInvalidMetadata
	}
	version := data[0]
	pos := 4
	countBytes := 2
	if version > 0 {
		countBytes = 4
	}
	count, err := readUint(data, &pos, countBytes)
	if err != nil || count > 4096 {
		return ErrInvalidMetadata
	}
	for range int(count) {
		size, header, err := boxSize(data, pos)
		if err != nil {
			return err
		}
		if string(data[pos+4:pos+8]) != "infe" {
			return ErrInvalidMetadata
		}
		payload := data[pos+header : pos+size]
		if len(payload) < 4 {
			return ErrInvalidMetadata
		}
		v := payload[0]
		if v != 2 && v != 3 {
			return ErrUnsupportedFormat
		}
		at := 4
		idSize := 2
		if v == 3 {
			idSize = 4
		}
		id, err := readUint(payload, &at, idSize)
		if err != nil {
			return err
		}
		if len(payload)-at < 6 {
			return ErrInvalidMetadata
		}
		at += 2
		typ := string(payload[at : at+4])
		at += 4
		kind := ""
		switch typ {
		case "Exif":
			kind = "exif"
		case "mime":
			nul := bytes.IndexByte(payload[at:], 0)
			if nul < 0 {
				return ErrInvalidMetadata
			}
			at += nul + 1
			nul = bytes.IndexByte(payload[at:], 0)
			if nul < 0 {
				return ErrInvalidMetadata
			}
			content := string(payload[at : at+nul])
			if content == "application/rdf+xml" || content == "application/xml" || content == "text/xml" {
				kind = "xmp"
			}
		}
		if _, exists := items[id]; exists {
			return ErrInvalidMetadata
		}
		items[id] = kind
		pos += size
	}
	if pos != len(data) {
		return ErrInvalidMetadata
	}
	return nil
}

func readItemLocations(data []byte, sourceSize, idata, idataEnd int, locations map[uint64]isoItem) error {
	if sourceSize < 0 || sourceSize > maxSourceBytes {
		return ErrInvalidMetadata
	}
	if len(data) < 8 {
		return ErrInvalidMetadata
	}
	version := data[0]
	if version > 2 {
		return ErrUnsupportedFormat
	}
	offsetSize, lengthSize := int(data[4]>>4), int(data[4]&15)
	baseSize, indexSize := int(data[5]>>4), 0
	if version > 0 {
		indexSize = int(data[5] & 15)
	}
	for _, size := range []int{offsetSize, lengthSize, baseSize, indexSize} {
		if size != 0 && size != 4 && size != 8 {
			return ErrUnsupportedFormat
		}
	}
	pos := 6
	idSize := 2
	if version == 2 {
		idSize = 4
	}
	count, err := readUint(data, &pos, idSize)
	if err != nil || count > 4096 {
		return ErrInvalidMetadata
	}
	for range int(count) {
		id, err := readUint(data, &pos, idSize)
		if err != nil {
			return err
		}
		method := uint64(0)
		if version > 0 {
			method, err = readUint(data, &pos, 2)
			if err != nil {
				return err
			}
			method &= 15
		}
		reference, err := readUint(data, &pos, 2)
		if err != nil {
			return err
		}
		if reference != 0 || method > 1 {
			return ErrUnsupportedFormat
		}
		base, err := readUint(data, &pos, baseSize)
		if err != nil {
			return err
		}
		if method == 1 {
			if idataEnd < 0 || idataEnd > sourceSize || idataEnd < idata {
				return ErrInvalidMetadata
			}
			if idata < 0 || idata > sourceSize || base > uint64(sourceSize)-uint64(idata) {
				return ErrInvalidMetadata
			}
			base += uint64(idata)
		}
		extents, err := readUint(data, &pos, 2)
		if err != nil || extents > 4096 {
			return ErrInvalidMetadata
		}
		item := isoItem{id: id, extents: []rawExtent{}}
		for range int(extents) {
			if _, err := readUint(data, &pos, indexSize); err != nil {
				return err
			}
			offset, err := readUint(data, &pos, offsetSize)
			if err != nil {
				return err
			}
			length, err := readUint(data, &pos, lengthSize)
			if err != nil {
				return err
			}
			if base > uint64(sourceSize) || offset > uint64(sourceSize)-base {
				return ErrInvalidMetadata
			}
			absolute := base + offset
			if length == 0 || length > uint64(sourceSize)-absolute {
				return ErrInvalidMetadata
			}
			if method == 1 {
				if idataEnd < 0 || idataEnd > maxSourceBytes {
					return ErrInvalidMetadata
				}
				endLimit := uint64(idataEnd)
				if absolute > endLimit || length > endLimit-absolute {
					return ErrInvalidMetadata
				}
			}
			start, err := boundedOffset(absolute)
			if err != nil {
				return err
			}
			boundedLength, err := boundedOffset(length)
			if err != nil {
				return err
			}
			item.extents = append(item.extents, rawExtent{Offset: start, Length: boundedLength})
		}
		if _, exists := locations[id]; exists {
			return ErrInvalidMetadata
		}
		locations[id] = item
	}
	if pos != len(data) {
		return ErrInvalidMetadata
	}
	return nil
}

func readUint(data []byte, pos *int, size int) (uint64, error) {
	if size < 0 || *pos > len(data) || size > len(data)-*pos {
		return 0, ErrInvalidMetadata
	}
	value := uint64(0)
	for _, b := range data[*pos : *pos+size] {
		value = value<<8 | uint64(b)
	}
	*pos += size
	return value, nil
}
