package native

import (
	"context"
	"errors"
	"github.com/biliblihuorong/imgnest/internal/http/reqbody"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
)

var errUploadTooLarge = errors.New("upload exceeds size limit")
var errUploadRateLimited = errors.New("upload rate exceeded")

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
	case <-ctx.Done():
		fail(c, ctx.Err())
		return
	}
	limits, err := h.images.Preflight(ctx, identity(c).Subject, 0)
	if err != nil {
		fail(c, err)
		return
	}
	if limits.MaxFileBytes <= 0 {
		fail(c, service.ErrInvalidInput)
		return
	}
	if c.Request.ContentLength > h.options.MaxRequestBytes {
		fail(c, errUploadTooLarge)
		return
	}
	defer reqbody.Bound(ctx, c.Writer, c.Request, h.options.MaxRequestBytes)()
	reader, err := c.Request.MultipartReader()
	if err != nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	files, fields, err := readUpload(ctx, reader, limits.MaxFileBytes, h.options.MaxRequestBytes)
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
	for _, file := range files {
		result := imageResult{Filename: file.name, Status: 201, Response: Response{Code: 0, Message: "ok"}}
		err := file.err
		if err == nil && int64(len(file.data)) > limits.MaxFileBytes {
			err = errUploadTooLarge
		}
		if err == nil && !h.allowUpload(identity(c).User.ID, limits.PerMinute) {
			err = errUploadRateLimited
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
			if len(files) >= 20 {
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
	return h.auth.limits.Allow("upload:"+strconv.FormatUint(userID, 10), perMinute)
}
