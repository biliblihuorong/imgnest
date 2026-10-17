package native

import (
	"context"
	"errors"
	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/http/reqbody"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

var errUploadTooLarge = errors.New("upload exceeds size limit")
var errUploadRateLimited = errors.New("upload rate exceeded")

// uploadSlotWait bounds how long an upload waits for a processing slot
// before answering with throttling instead of holding the caller for the
// whole processing timeout.
const uploadSlotWait = 10 * time.Second

type uploadFile struct {
	name string
	data []byte
	err  error
}
type uploadFields struct {
	policyID, albumID uint64
	public            bool
}

func (h *imageHandler) upload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.options.Timeout)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-time.After(uploadSlotWait):
		fail(c, errUploadRateLimited)
		return
	case <-ctx.Done():
		fail(c, ctx.Err())
		return
	}
	// The first gate only bounds the body read: the group's default rule may
	// be unusable while the request names an explicit, usable rule in its
	// fields, so the terminal rule check waits for the second preflight.
	limits, err := h.images.PreflightLimits(ctx, identity(c).Subject)
	if err != nil {
		fail(c, err)
		return
	}
	if limits.MaxFileBytes <= 0 {
		fail(c, service.ErrInvalidInput)
		return
	}
	userID := identity(c).User.ID
	if limits.PerMinute > 0 && h.options.Uploads.Exceeded(ratelimit.UploadKey(userID), limits.PerMinute) {
		// An exhausted allowance is refused before the body is read.
		fail(c, errUploadRateLimited)
		return
	}
	requestLimit := reqbody.Fit(h.options.MaxRequestBytes, limits.MaxFileBytes)
	if c.Request.ContentLength > requestLimit {
		fail(c, errUploadTooLarge)
		return
	}
	defer reqbody.Bound(ctx, c.Writer, c.Request, requestLimit)()
	reader, err := c.Request.MultipartReader()
	if err != nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	// Reserve the first file's attempt before reading or draining the body.
	// Per-file rejection, epilogue errors and an unusable selected rule still
	// spend this allowance; cheap header-only rejections above do not.
	if !h.allowUpload(userID, limits.PerMinute) {
		fail(c, errUploadRateLimited)
		return
	}
	files, fields, err := readUpload(ctx, reader, limits.MaxFileBytes, requestLimit)
	if err != nil {
		fail(c, err)
		return
	}
	// Multipart epilogues are still part of the bounded request body.
	if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
		fail(c, uploadReadError(ctx, err))
		return
	}
	limits, err = h.images.Preflight(ctx, identity(c).Subject, fields.policyID)
	if err != nil {
		fail(c, err)
		return
	}
	results := make([]imageResult, 0, len(files))
	for index, file := range files {
		result := imageResult{Filename: file.name, Status: 201, Response: Response{Code: 0, Message: "ok"}}
		err := file.err
		// The first attempt was reserved before parsing. Every additional
		// batch file spends its own attempt, including rejected files.
		if index > 0 && !h.allowUpload(userID, limits.PerMinute) {
			err = errUploadRateLimited
		}
		if err == nil && int64(len(file.data)) > limits.MaxFileBytes {
			err = errUploadTooLarge
		}
		if err == nil {
			result.Data, err = h.images.Upload(ctx, identity(c).Subject, service.UploadInput{Data: file.data, Filename: file.name, IP: c.ClientIP(), PolicyID: fields.policyID, AlbumID: fields.albumID, IsPublic: fields.public})
		}
		if err != nil {
			result.Status, result.Response = errorResponse(err)
		}
		results = append(results, result)
	}
	if len(results) == 1 {
		c.JSON(results[0].Status, results[0].Response)
		return
	}
	respond(c, 207, results)
}

// uploadLimitsView is what a client checks files against before sending
// them: the caller's per-file limit and how many files one request takes.
type uploadLimitsView struct {
	MaxFileBytes int64 `json:"max_file_bytes"`
	MaxFiles     int   `json:"max_files"`
	UploadPerMin int   `json:"upload_per_min"`
}

func (h *imageHandler) uploadLimits(c *gin.Context) {
	limits, err := h.images.UploadLimitsFor(c.Request.Context(), identity(c).Subject)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, uploadLimitsView{MaxFileBytes: limits.MaxFileBytes, MaxFiles: maxUploadFiles, UploadPerMin: limits.PerMinute})
}

// maxUploadFiles is how many files one multipart request may carry.
const maxUploadFiles = 20

func readUpload(ctx context.Context, reader *multipart.Reader, fileLimit, requestLimit int64) ([]uploadFile, uploadFields, error) {
	var files []uploadFile
	fields := uploadFields{}
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, fields, err
		}
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fields, uploadReadError(ctx, err)
		}
		name := part.FormName()
		if name == "file" || name == "files[]" {
			if len(files) >= maxUploadFiles {
				_ = part.Close()
				return nil, fields, service.ErrInvalidInput
			}
			limit := min(fileLimit, requestLimit)
			data, readErr := io.ReadAll(io.LimitReader(part, limit+1))
			if readErr != nil {
				_ = part.Close()
				return nil, fields, uploadReadError(ctx, readErr)
			}
			file := uploadFile{name: part.FileName(), data: data}
			if file.name == "" {
				file.err = service.ErrInvalidInput
				file.data = nil
			}
			if int64(len(data)) > limit {
				file.err = errUploadTooLarge
				file.data = nil
			}
			if _, err := io.Copy(io.Discard, part); err != nil {
				_ = part.Close()
				return nil, fields, uploadReadError(ctx, err)
			}
			if err := part.Close(); err != nil {
				return nil, fields, uploadReadError(ctx, err)
			}
			files = append(files, file)
			continue
		}
		if seen[name] || (name != "policy_id" && name != "album_id" && name != "is_public") {
			_ = part.Close()
			return nil, fields, service.ErrInvalidInput
		}
		seen[name] = true
		data, err := io.ReadAll(io.LimitReader(part, 1025))
		if err != nil {
			_ = part.Close()
			return nil, fields, uploadReadError(ctx, err)
		}
		if len(data) > 1024 {
			_ = part.Close()
			return nil, fields, service.ErrInvalidInput
		}
		if err := part.Close(); err != nil {
			return nil, fields, uploadReadError(ctx, err)
		}
		switch name {
		case "policy_id", "album_id":
			id, err := parseImageID(string(data))
			if err != nil {
				return nil, fields, err
			}
			if name == "policy_id" {
				fields.policyID = id
			} else {
				fields.albumID = id
			}
		case "is_public":
			if string(data) != "true" && string(data) != "false" {
				return nil, fields, service.ErrInvalidInput
			}
			fields.public = string(data) == "true"
		}
	}
	if len(files) == 0 {
		return nil, fields, service.ErrInvalidInput
	}
	return files, fields, nil
}
func uploadReadError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errUploadTooLarge
	}
	return service.ErrInvalidInput
}
func (h *imageHandler) allowUpload(userID uint64, perMinute int) bool {
	return h.options.Uploads.Allow(ratelimit.UploadKey(userID), perMinute)
}
