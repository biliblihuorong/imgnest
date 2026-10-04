// Package exif archives original metadata and edits it without reencoding pixels.
package exif

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/meta/logging"
)

// Extractor preserves original metadata and exposes common owner-only fields.
type Extractor interface {
	Extract(context.Context, []byte, imaging.Info) (model.ImageExif, error)
}

// Scrubber removes sensitive metadata from original compressed image buffers.
type Scrubber interface {
	Scrub(context.Context, []byte, string, string) ([]byte, error)
}

// Metadata errors contain no original values.
var (
	ErrInvalidMetadata   = errors.New("invalid image metadata")
	ErrUnsupportedFormat = errors.New("unsupported metadata format")
)

// Processor extracts and scrubs bounded container metadata.
type Processor struct{}

var loggingOnce sync.Once

// NewProcessor creates an extractor and original-image scrubber.
func NewProcessor(ctx context.Context) (*Processor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create metadata processor: %w", err)
	}
	loggingOnce.Do(func() { imagemeta.SetLogger(io.Discard, logging.LevelDisabled) })
	return &Processor{}, nil
}

// Extract archives original bytes before any privacy edits.
func (p *Processor) Extract(ctx context.Context, data []byte, info imaging.Info) (model.ImageExif, error) {
	if err := ctx.Err(); err != nil {
		return model.ImageExif{}, fmt.Errorf("extract metadata: %w", err)
	}
	if len(data) == 0 || len(data) > maxSourceBytes {
		return model.ImageExif{}, ErrInvalidMetadata
	}
	parts, err := scanContainer(data, info.Format)
	if err != nil {
		return model.ImageExif{}, err
	}
	archive := rawArchive{Version: 1, Format: info.Format, CaptureMode: "metadata-blocks", Blocks: []rawBlock{}}
	out := model.ImageExif{Orientation: info.Orientation}
	if out.Orientation == 0 {
		out.Orientation = 1
	}
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return model.ImageExif{}, fmt.Errorf("extract metadata: %w", err)
		}
		if part.kind == "tiff-container" {
			doc, err := parseTIFF(part.payload)
			if err != nil {
				return model.ImageExif{}, err
			}
			if doc.private {
				archive.CaptureMode = "full-source-fallback"
				archive.Blocks = append(archive.Blocks, rawBlock{Kind: "tiff-container", Offset: 0, Order: len(archive.Blocks), Data: bytes.Clone(data)})
			} else {
				archive.Blocks = append(archive.Blocks, doc.archive()...)
			}
			if err := decodeCommon(part.payload, doc, &out); err != nil {
				return model.ImageExif{}, err
			}
			continue
		}
		archive.Blocks = append(archive.Blocks, rawBlock{Kind: part.kind, Offset: part.start, Order: len(archive.Blocks), Data: bytes.Clone(data[part.start:part.end])})
		if part.kind == "exif" {
			payload := tiffPayload(part.payload)
			doc, err := parseTIFF(payload)
			if err != nil {
				return model.ImageExif{}, err
			}
			if err := decodeCommon(payload, doc, &out); err != nil {
				return model.ImageExif{}, err
			}
		}
	}
	if info.Format == "heic" || info.Format == "heif" || info.Format == "avif" {
		archive.Blocks, err = isobmffBlocks(data)
		if err != nil {
			return model.ImageExif{}, err
		}
		archive.CaptureMode = "isobmff-items"
		for _, block := range archive.Blocks {
			if block.Kind == "exif" {
				payload := block.Data[block.TIFFOffset:]
				doc, err := parseTIFF(payload)
				if err != nil {
					return model.ImageExif{}, err
				}
				if err := decodeCommon(payload, doc, &out); err != nil {
					return model.ImageExif{}, err
				}
			}
		}
	}
	for i := range archive.Blocks {
		archive.Blocks[i].Order = i
	}
	out.Raw, err = json.Marshal(archive)
	if err != nil {
		return model.ImageExif{}, ErrInvalidMetadata
	}
	return out, nil
}

// Scrub returns a separate buffer with privacy edits and untouched compressed pixels.
func (p *Processor) Scrub(ctx context.Context, data []byte, format, mode string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("scrub metadata: %w", err)
	}
	if mode != "none" && mode != "gps" && mode != "all" {
		return nil, ErrInvalidMetadata
	}
	if len(data) == 0 || len(data) > maxSourceBytes {
		return nil, ErrInvalidMetadata
	}
	parts, err := scanContainer(data, format)
	if err != nil {
		return nil, err
	}
	if mode == "none" {
		return bytes.Clone(data), nil
	}
	if format == "heic" || format == "heif" || format == "avif" {
		return nil, ErrUnsupportedFormat
	}
	if format == "tiff" {
		doc, err := parseTIFF(data)
		if err != nil {
			return nil, err
		}
		if doc.private {
			return nil, ErrUnsupportedFormat
		}
		return doc.scrub(mode, true)
	}
	out := []byte{}
	position := 0
	removedXMP := false
	for _, part := range parts {
		out = append(out, data[position:part.start]...)
		position = part.end
		if part.kind == "xmp" || part.kind == "xmp-extended" {
			removedXMP = true
			continue
		}
		if mode == "all" && part.kind == "gif-comment" {
			continue
		}
		if part.kind != "exif" {
			out = append(out, data[part.start:part.end]...)
			continue
		}
		doc, err := parseTIFF(tiffPayload(part.payload))
		if err != nil {
			return nil, err
		}
		var payload []byte
		if mode == "all" {
			payload = minimalTIFF(doc.orientation)
		} else {
			payload, err = doc.scrub(mode, false)
			if err != nil {
				return nil, err
			}
		}
		switch format {
		case "jpeg":
			payload = append([]byte("Exif\x00\x00"), payload...)
			if len(payload) > 65533 {
				return nil, ErrInvalidMetadata
			}
			out = append(out, 0xff, 0xe1, byte(((len(payload)+2)>>8)&255), byte((len(payload)+2)&255))
			out = append(out, payload...)
		case "png":
			out = append(out, pngChunkBytes("eXIf", payload)...)
		case "webp":
			out = append(out, riffChunkBytes("EXIF", payload)...)
		default:
			return nil, ErrUnsupportedFormat
		}
	}
	out = append(out, data[position:]...)
	if format == "webp" {
		if len(out) < 12 {
			return nil, ErrInvalidMetadata
		}
		binaryPutRIFFSize(out)
		if removedXMP {
			for pos := 12; pos+8 <= len(out); {
				size := int(little.Uint32(out[pos+4:]))
				if string(out[pos:pos+4]) == "VP8X" && size >= 1 {
					out[pos+8] &^= 4
				}
				pos += 8 + size + (size % 2)
			}
		}
	}
	if _, err := scanContainer(out, format); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("scrub metadata: %w", err)
	}
	return out, nil
}

const maxSourceBytes = 20 << 20

type rawArchive struct {
	Version     int        `json:"version"`
	Format      string     `json:"format"`
	CaptureMode string     `json:"capture_mode"`
	Blocks      []rawBlock `json:"blocks"`
}
type rawBlock struct {
	Kind       string      `json:"kind"`
	Offset     int         `json:"offset"`
	Order      int         `json:"order"`
	Data       []byte      `json:"data"`
	TIFFOffset int         `json:"tiff_offset,omitempty"`
	Extents    []rawExtent `json:"extents,omitempty"`
}
type rawExtent struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

func decodeCommon(payload []byte, doc *tiffDocument, out *model.ImageExif) error {
	decoded, err := imagemeta.DecodeTiff(bytes.NewReader(payload))
	if err != nil && !errors.Is(err, imagemeta.ErrNoExif) {
		return ErrInvalidMetadata
	}
	if errors.Is(err, imagemeta.ErrNoExif) {
		return nil
	}
	out.Make = decoded.IFD0.Make
	out.Model = decoded.IFD0.Model
	out.Lens = decoded.ExifIFD.LensModel
	out.Exposure = decoded.ExifIFD.ExposureTime.String()
	out.FNumber = decoded.ExifIFD.FNumber.String()
	out.FocalLength = decoded.ExifIFD.FocalLength.String()
	out.ISO = int(decoded.ExifIFD.ISOSpeedRatings)
	// The library canonicalizes camera makes; retain the original standard tag text.
	for _, dir := range doc.dirs {
		for _, entry := range dir.entries {
			if entry.id == 0x10f && entry.kind == 2 {
				out.Make = strings.TrimRight(string(doc.data[entry.offset:entry.offset+entry.size]), "\x00")
			}
		}
	}
	if doc.orientation != 0 {
		out.Orientation = doc.orientation
	}
	if at := decoded.SelectedDate(); !at.IsZero() {
		utc := at.UTC()
		out.TakenAt = &utc
	}
	if doc.gpsLat {
		v := decoded.GPS.Latitude()
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -90 || v > 90 {
			return ErrInvalidMetadata
		}
		out.GPSLat = &v
	}
	if doc.gpsLng {
		v := decoded.GPS.Longitude()
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -180 || v > 180 {
			return ErrInvalidMetadata
		}
		out.GPSLng = &v
	}
	if doc.gpsAlt {
		v := float64(decoded.GPS.Altitude())
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalidMetadata
		}
		out.GPSAlt = &v
	}
	return nil
}
