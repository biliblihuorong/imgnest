package exif

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

var little = binary.LittleEndian

type containerPart struct {
	kind       string
	start, end int
	payload    []byte
}

func scanContainer(data []byte, format string) ([]containerPart, error) {
	switch format {
	case "jpeg":
		return scanJPEG(data)
	case "png":
		return scanPNG(data)
	case "webp":
		return scanWebP(data)
	case "gif":
		return scanGIF(data)
	case "tiff":
		if _, err := parseTIFF(data); err != nil {
			return nil, err
		}
		return []containerPart{{kind: "tiff-container", start: 0, end: len(data), payload: data}}, nil
	case "heic", "heif", "avif":
		if len(data) < 16 || string(data[4:8]) != "ftyp" {
			return nil, ErrInvalidMetadata
		}
		for pos := 0; pos < len(data); {
			size, _, err := boxSize(data, pos)
			if err != nil {
				return nil, err
			}
			pos += size
		}
		return []containerPart{}, nil
	case "bmp":
		if len(data) < 54 || string(data[:2]) != "BM" {
			return nil, ErrInvalidMetadata
		}
		parts := []containerPart{}
		if len(data) >= 138 && little.Uint32(data[14:18]) == 124 {
			offset := uint64(little.Uint32(data[126:130])) + 14
			size := uint64(little.Uint32(data[130:134]))
			if size > 0 {
				if offset > uint64(len(data)) || size > uint64(len(data))-offset {
					return nil, ErrInvalidMetadata
				}
				start, err := boundedOffset(offset)
				if err != nil {
					return nil, err
				}
				end, err := boundedOffset(offset + size)
				if err != nil {
					return nil, err
				}
				parts = append(parts, containerPart{kind: "icc", start: start, end: end, payload: data[start:end]})
			}
		}
		return parts, nil
	default:
		return nil, ErrUnsupportedFormat
	}
}

func scanJPEG(data []byte) ([]containerPart, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, ErrInvalidMetadata
	}
	parts := []containerPart{}
	for pos := 2; pos < len(data); {
		start := pos
		if data[pos] != 0xff {
			return nil, ErrInvalidMetadata
		}
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			return nil, ErrInvalidMetadata
		}
		marker := data[pos]
		pos++
		if marker == 0xda {
			if pos+2 > len(data) {
				return nil, ErrInvalidMetadata
			}
			size := int(binary.BigEndian.Uint16(data[pos:]))
			if size < 2 || size > len(data)-pos {
				return nil, ErrInvalidMetadata
			}
			pos += size
			for pos < len(data) {
				if data[pos] != 0xff {
					pos++
					continue
				}
				next := pos + 1
				for next < len(data) && data[next] == 0xff {
					next++
				}
				if next >= len(data) {
					return nil, ErrInvalidMetadata
				}
				code := data[next]
				if code == 0 || code >= 0xd0 && code <= 0xd7 {
					pos = next + 1
					continue
				}
				break
			}
			continue
		}
		if marker == 0xd9 {
			if pos != len(data) {
				return nil, ErrInvalidMetadata
			}
			return parts, nil
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if pos+2 > len(data) {
			return nil, ErrInvalidMetadata
		}
		size := int(binary.BigEndian.Uint16(data[pos:]))
		if size < 2 || size > len(data)-pos {
			return nil, ErrInvalidMetadata
		}
		end := pos + size
		payload := data[pos+2 : end]
		kind := ""
		if marker == 0xe1 {
			switch {
			case bytes.HasPrefix(payload, []byte("Exif\x00\x00")):
				kind = "exif"
			case bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xap/1.0/\x00")):
				kind = "xmp"
			case bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xmp/extension/\x00")):
				kind = "xmp-extended"
			default:
				kind = "jpeg-app1-unknown"
			}
		}
		if marker == 0xe2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
			kind = "icc"
		}
		if kind != "" {
			parts = append(parts, containerPart{kind: kind, start: start, end: end, payload: payload})
		}
		pos = end
	}
	return nil, ErrInvalidMetadata
}

func scanPNG(data []byte) ([]containerPart, error) {
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, ErrInvalidMetadata
	}
	parts := []containerPart{}
	for pos := 8; pos < len(data); {
		if len(data)-pos < 12 {
			return nil, ErrInvalidMetadata
		}
		size, err := boundedOffset(uint64(binary.BigEndian.Uint32(data[pos:])))
		if err != nil {
			return nil, err
		}
		if size > len(data)-pos-12 {
			return nil, ErrInvalidMetadata
		}
		end := pos + 12 + size
		if crc32.ChecksumIEEE(data[pos+4:end-4]) != binary.BigEndian.Uint32(data[end-4:end]) {
			return nil, ErrInvalidMetadata
		}
		name := string(data[pos+4 : pos+8])
		payload := data[pos+8 : end-4]
		kind := ""
		switch name {
		case "eXIf":
			kind = "exif"
		case "iCCP":
			kind = "icc"
		case "iTXt", "tEXt", "zTXt":
			if bytes.HasPrefix(payload, []byte("XML:com.adobe.xmp\x00")) {
				kind = "xmp"
			} else {
				kind = "png-text"
			}
		}
		if kind != "" {
			parts = append(parts, containerPart{kind: kind, start: pos, end: end, payload: payload})
		}
		pos = end
		if name == "IEND" {
			if pos != len(data) {
				return nil, ErrInvalidMetadata
			}
			return parts, nil
		}
	}
	return nil, ErrInvalidMetadata
}

func scanWebP(data []byte) ([]containerPart, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || uint64(little.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return nil, ErrInvalidMetadata
	}
	parts := []containerPart{}
	for pos := 12; pos < len(data); {
		if len(data)-pos < 8 {
			return nil, ErrInvalidMetadata
		}
		size, err := boundedOffset(uint64(little.Uint32(data[pos+4:])))
		if err != nil {
			return nil, err
		}
		padded := size + size%2
		if padded > len(data)-pos-8 {
			return nil, ErrInvalidMetadata
		}
		end := pos + 8 + padded
		payload := data[pos+8 : pos+8+size]
		kind := ""
		switch string(data[pos : pos+4]) {
		case "EXIF":
			kind = "exif"
		case "XMP ":
			kind = "xmp"
		case "ICCP":
			kind = "icc"
		}
		if kind != "" {
			parts = append(parts, containerPart{kind: kind, start: pos, end: end, payload: payload})
		}
		pos = end
	}
	return parts, nil
}

func scanGIF(data []byte) ([]containerPart, error) {
	if len(data) < 13 || string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return nil, ErrInvalidMetadata
	}
	parts := []containerPart{}
	pos := 13
	if data[10]&128 != 0 {
		pos += 3 * (1 << ((data[10] & 7) + 1))
	}
	if pos > len(data) {
		return nil, ErrInvalidMetadata
	}
	for pos < len(data) {
		start := pos
		marker := data[pos]
		pos++
		switch marker {
		case 0x3b:
			if pos != len(data) {
				return nil, ErrInvalidMetadata
			}
			return parts, nil
		case 0x2c:
			if len(data)-pos < 9 {
				return nil, ErrInvalidMetadata
			}
			packed := data[pos+8]
			pos += 9
			if packed&128 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			if pos >= len(data) {
				return nil, ErrInvalidMetadata
			}
			pos++
			end, _, err := gifSubBlocks(data, pos)
			if err != nil {
				return nil, err
			}
			pos = end
		case 0x21:
			if pos >= len(data) {
				return nil, ErrInvalidMetadata
			}
			label := data[pos]
			pos++
			end, payload, err := gifSubBlocks(data, pos)
			if err != nil {
				return nil, err
			}
			kind := ""
			switch label {
			case 0xff:
				kind = "gif-application"
				if bytes.HasPrefix(payload, []byte("XMP DataXMP")) {
					kind = "xmp"
				}
			case 0xfe:
				kind = "gif-comment"
			}
			if kind != "" {
				parts = append(parts, containerPart{kind: kind, start: start, end: end, payload: payload})
			}
			pos = end
		default:
			return nil, ErrInvalidMetadata
		}
	}
	return nil, ErrInvalidMetadata
}

func gifSubBlocks(data []byte, pos int) (int, []byte, error) {
	out := []byte{}
	for pos < len(data) {
		size := int(data[pos])
		pos++
		if size == 0 {
			return pos, out, nil
		}
		if size > len(data)-pos {
			return 0, nil, ErrInvalidMetadata
		}
		out = append(out, data[pos:pos+size]...)
		pos += size
	}
	return 0, nil, ErrInvalidMetadata
}
func tiffPayload(data []byte) []byte {
	if bytes.HasPrefix(data, []byte("Exif\x00\x00")) {
		return data[6:]
	}
	return data
}
func pngChunkBytes(name string, data []byte) []byte {
	if len(data) > maxSourceBytes {
		return nil
	}
	out := make([]byte, 8)
	length := uint64(len(data))
	if length > maxSourceBytes {
		return nil
	}
	binary.BigEndian.PutUint32(out, uint32(length))
	copy(out[4:], name)
	out = append(out, data...)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint32(tail, crc32.ChecksumIEEE(out[4:]))
	return append(out, tail...)
}
func riffChunkBytes(name string, data []byte) []byte {
	if len(data) > maxSourceBytes {
		return nil
	}
	out := make([]byte, 8)
	copy(out, name)
	length := uint64(len(data))
	if length > maxSourceBytes {
		return nil
	}
	little.PutUint32(out[4:], uint32(length))
	out = append(out, data...)
	if len(data)%2 != 0 {
		out = append(out, 0)
	}
	return out
}
func binaryPutRIFFSize(data []byte) {
	if len(data) < 8 || len(data) > maxSourceBytes {
		return
	}
	length := uint64(len(data)) - 8
	if length > maxSourceBytes {
		return
	}
	little.PutUint32(data[4:8], uint32(length))
}
func boxSize(data []byte, pos int) (int, int, error) {
	if len(data)-pos < 8 {
		return 0, 0, ErrInvalidMetadata
	}
	size := uint64(binary.BigEndian.Uint32(data[pos:]))
	header := 8
	switch size {
	case 1:
		if len(data)-pos < 16 {
			return 0, 0, ErrInvalidMetadata
		}
		size = binary.BigEndian.Uint64(data[pos+8:])
		header = 16
	case 0:
		if pos < 0 || pos > len(data) {
			return 0, 0, ErrInvalidMetadata
		}
		size = uint64(len(data)) - uint64(pos)
	}
	bounded, err := boundedOffset(size)
	if err != nil {
		return 0, 0, err
	}
	if bounded < header || bounded > len(data)-pos {
		return 0, 0, ErrInvalidMetadata
	}
	return bounded, header, nil
}

func boundedOffset(value uint64) (int, error) {
	if value > maxSourceBytes {
		return 0, ErrInvalidMetadata
	}
	return int(value), nil
}
