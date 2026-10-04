// Package imaging probes and processes bounded image buffers.
package imaging

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/cshum/vipsgen/vips"
)

// Info describes the true format, visual dimensions, and loaded frames.
type Info struct {
	Format, Ext, MIME                        string
	Width, Height, LoadedFrames, Orientation int
}

// Options contains the per-policy derivative settings.
type Options struct {
	WebPMode                                        string
	Quality, Effort, MaxWidth, MaxHeight, ThumbSize int
	Lossless, SkipIfLarger, ThumbEnabled            bool
}

// Result contains safe derivatives and the source image information.
type Result struct {
	Info            Info
	WebP, Thumbnail []byte
}

// Processor provides bounded image validation and synchronous processing.
type Processor interface {
	Probe(context.Context, []byte) (Info, error)
	Process(context.Context, []byte, Options) (Result, error)
}

// Image processing errors deliberately contain no source metadata.
var (
	ErrInvalidImage   = errors.New("invalid image")
	ErrPixelLimit     = errors.New("image pixel limit exceeded")
	ErrInvalidOptions = errors.New("invalid imaging options")
)

// VipsProcessor owns request concurrency while libvips owns its global lifecycle.
type VipsProcessor struct {
	maxPixels int64
	slots     chan struct{}
}

var startupOnce sync.Once
var startupErr error

// NewProcessor creates a processor with a pixel budget and concurrent request limit.
func NewProcessor(ctx context.Context, maxPixels int64, concurrency int) (*VipsProcessor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create imaging processor: %w", err)
	}
	if maxPixels <= 0 || concurrency <= 0 {
		return nil, ErrInvalidOptions
	}
	startupOnce.Do(func() {
		defer func() {
			if recover() != nil {
				startupErr = errors.New("image runtime initialization failed")
			}
		}()
		vips.Startup(&vips.Config{ConcurrencyLevel: 1, MaxCacheFiles: 0, MaxCacheMem: 0, MaxCacheSize: 0})
	})
	if startupErr != nil {
		return nil, startupErr
	}
	return &VipsProcessor{maxPixels: maxPixels, slots: make(chan struct{}, concurrency)}, nil
}

// Probe validates a source buffer and reports its visual information.
func (p *VipsProcessor) Probe(ctx context.Context, data []byte) (Info, error) {
	if err := p.acquire(ctx); err != nil {
		return Info{}, err
	}
	defer p.release()
	info, err := p.probe(data)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Info{}, fmt.Errorf("probe image: %w", ctxErr)
	}
	return info, err
}

// Process produces configured derivatives without modifying the source buffer.
func (p *VipsProcessor) Process(ctx context.Context, data []byte, opts Options) (Result, error) {
	if err := p.acquire(ctx); err != nil {
		return Result{}, err
	}
	defer p.release()
	if opts.WebPMode != "both" && opts.WebPMode != "webp_only" && opts.WebPMode != "none" {
		return Result{}, ErrInvalidOptions
	}
	if opts.Quality == 0 {
		opts.Quality = 80
	}
	if opts.ThumbSize == 0 {
		opts.ThumbSize = 400
	}
	invalidQuality := opts.Quality < 1 || opts.Quality > 100
	invalidEffort := opts.Effort < 0 || opts.Effort > 6
	invalidSize := opts.MaxWidth < 0 || opts.MaxHeight < 0 || opts.ThumbSize < 1
	if invalidQuality || invalidEffort || invalidSize {
		return Result{}, ErrInvalidOptions
	}
	info, err := p.probe(data)
	if err != nil {
		return Result{}, err
	}
	result := Result{Info: info, WebP: []byte{}, Thumbnail: []byte{}}
	if opts.WebPMode != "none" && info.Format != "webp" {
		width, height := opts.MaxWidth, opts.MaxHeight
		if width == 0 {
			width = info.Width
		}
		if height == 0 {
			height = info.Height
		}
		load := &vips.ThumbnailBufferOptions{Height: height, Size: vips.SizeDown, FailOn: vips.FailOnError}
		if info.Format == "gif" {
			load.OptionString = "n=-1"
		}
		save := vips.DefaultWebpsaveBufferOptions()
		save.Q = opts.Quality
		save.Effort = opts.Effort
		save.Lossless = opts.Lossless
		save.Keep = vips.KeepIcc
		result.WebP, err = encode(data, width, load, save)
		if err != nil {
			return Result{}, err
		}
		if opts.WebPMode == "both" && opts.SkipIfLarger && len(result.WebP) > len(data) {
			result.WebP = []byte{}
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("process image: %w", err)
	}
	if opts.ThumbEnabled {
		save := vips.DefaultWebpsaveBufferOptions()
		save.Q = 75
		save.Effort = 4
		save.Keep = vips.KeepNone
		result.Thumbnail, err = encode(data, opts.ThumbSize, &vips.ThumbnailBufferOptions{Height: opts.ThumbSize, Size: vips.SizeDown, FailOn: vips.FailOnError}, save)
		if err != nil {
			return Result{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("process image: %w", err)
	}
	return result, nil
}

func (p *VipsProcessor) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("wait for image processor: %w", err)
	}
	select {
	case p.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			p.release()
			return fmt.Errorf("wait for image processor: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for image processor: %w", ctx.Err())
	}
}
func (p *VipsProcessor) release() { <-p.slots }

func (p *VipsProcessor) probe(data []byte) (Info, error) {
	format, ext, mime := magic(data)
	if format == "" {
		return Info{}, ErrInvalidImage
	}
	// Some loaders repair absent terminators; upload validation must reject those sources.
	switch format {
	case "jpeg":
		if data[len(data)-2] != 0xff || data[len(data)-1] != 0xd9 {
			return Info{}, ErrInvalidImage
		}
	case "gif":
		if data[len(data)-1] != 0x3b {
			return Info{}, ErrInvalidImage
		}
	case "png":
		if len(data) < 20 || string(data[len(data)-8:len(data)-4]) != "IEND" || binary.BigEndian.Uint32(data[len(data)-12:]) != 0 {
			return Info{}, ErrInvalidImage
		}
	}
	load := &vips.LoadOptions{FailOn: vips.FailOnError}
	if format == "gif" || format == "webp" {
		load.N = -1
	}
	img, err := vips.NewImageFromBuffer(data, load)
	if err != nil {
		return Info{}, ErrInvalidImage
	}
	defer img.Close()
	w, h, ph := img.Width(), img.Height(), img.PageHeight()
	if w <= 0 || h <= 0 || ph <= 0 || h%ph != 0 {
		return Info{}, ErrInvalidImage
	}
	frames := h / ph
	if int64(w) > p.maxPixels/int64(ph) || int64(w)*int64(ph) > p.maxPixels/int64(frames) {
		return Info{}, ErrPixelLimit
	}
	orientation := 1
	if img.HasField("orientation") {
		orientation, err = img.GetInt("orientation")
		if err != nil || orientation < 1 || orientation > 8 {
			return Info{}, ErrInvalidImage
		}
	}
	// Force bounded decoding: header-only loaders can otherwise accept truncated pixels.
	if _, err := img.Avg(); err != nil {
		return Info{}, ErrInvalidImage
	}
	height := ph
	if orientation >= 5 {
		w, height = height, w
	}
	return Info{Format: format, Ext: ext, MIME: mime, Width: w, Height: height, LoadedFrames: frames, Orientation: orientation}, nil
}

func encode(data []byte, width int, load *vips.ThumbnailBufferOptions, save *vips.WebpsaveBufferOptions) ([]byte, error) {
	img, err := vips.NewThumbnailBuffer(data, width, load)
	if err != nil {
		return nil, ErrInvalidImage
	}
	defer img.Close()
	out, err := img.WebpsaveBuffer(save)
	if err != nil {
		return nil, ErrInvalidImage
	}
	return out, nil
}

func magic(data []byte) (format, ext, mime string) {
	if len(data) < 8 {
		return "", "", ""
	}
	switch {
	case data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpeg", "jpg", "image/jpeg"
	case string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "png", "png", "image/png"
	case string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a":
		return "gif", "gif", "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		if uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
			return "", "", ""
		}
		return "webp", "webp", "image/webp"
	case string(data[:2]) == "BM":
		return "bmp", "bmp", "image/bmp"
	case string(data[:4]) == "II*\x00" || string(data[:4]) == "MM\x00*" || string(data[:4]) == "II+\x00" || string(data[:4]) == "MM\x00+":
		return "tiff", "tiff", "image/tiff"
	case len(data) >= 16 && string(data[4:8]) == "ftyp":
		end := int(binary.BigEndian.Uint32(data[:4]))
		if end < 16 || end > len(data) {
			return "", "", ""
		}
		for i := 8; i+4 <= end; i += 4 {
			brand := string(data[i : i+4])
			if brand == "avif" || brand == "avis" {
				return "avif", "avif", "image/avif"
			}
		}
		for i := 8; i+4 <= end; i += 4 {
			brand := string(data[i : i+4])
			if brand == "heic" || brand == "heix" || brand == "hevc" || brand == "hevx" {
				return "heic", "heic", "image/heic"
			}
			if brand == "mif1" || brand == "msf1" {
				format = "heif"
			}
		}
		if format != "" {
			return "heif", "heif", "image/heif"
		}
	}
	return "", "", ""
}
