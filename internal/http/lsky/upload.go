package lsky

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/http/reqbody"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Parse and validation failures carry distinct client-facing messages; all of
// them answer HTTP 200 with status false except authentication.
var (
	errV1MissingFile    = errors.New("missing file")
	errV1MultipleFiles  = errors.New("multiple files")
	errV1InvalidField   = errors.New("invalid field")
	errV1TooLarge       = errors.New("upload exceeds size limit")
	errRateLimited      = errors.New("upload rate exceeded")
	errGuestAlbum       = errors.New("guest cannot use albums")
	errAlbumUnavailable = errors.New("album unavailable")
)

type uploadFile struct {
	name string
	data []byte
	err  error
}

type uploadFields struct {
	strategyID, albumID uint64
	public              bool
}

// upload serves POST /api/v1/upload for authenticated users and, when the
// administrator enabled it, for guests without an Authorization header.
func (h *Handler) upload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.options.Timeout)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-ctx.Done():
		c.JSON(200, failure("请求超时或已取消"))
		return
	}

	guest := c.GetHeader("Authorization") == ""
	strategyID := uint64(0)
	subject, userID, limits, err := h.preupload(ctx, c, guest, strategyID)
	if err != nil {
		h.uploadFailure(c, err)
		return
	}

	if c.Request.ContentLength > h.options.MaxRequestBytes {
		h.uploadFailure(c, errV1TooLarge)
		return
	}
	// Without a read deadline a slow body would hold the upload slot past the
	// timeout; anonymous callers reach this path when guest uploads are on.
	defer reqbody.Bound(ctx, c.Writer, c.Request, h.options.MaxRequestBytes)()
	reader, err := c.Request.MultipartReader()
	if err != nil {
		h.uploadFailure(c, errV1MissingFile)
		return
	}
	file, fields, err := readV1Upload(ctx, reader, limits.MaxFileBytes, h.options.MaxRequestBytes)
	if err == nil {
		err = file.err
	}
	if err == nil {
		// Re-run the gate with the caller-selected rule before publishing.
		strategyID = fields.strategyID
		subject, userID, limits, err = h.preupload(ctx, c, guest, strategyID)
	}
	if err == nil {
		err = h.checkUploadFields(ctx, guest, userID, c.ClientIP(), fields, limits)
	}
	if err != nil {
		h.uploadFailure(c, err)
		return
	}

	input := service.UploadInput{Data: file.data, Filename: file.name, IP: c.ClientIP(), PolicyID: fields.strategyID, AlbumID: fields.albumID, IsPublic: fields.public}
	var view service.ImageView
	if guest {
		view, err = h.images.GuestUpload(ctx, input)
	} else {
		view, err = h.images.Upload(ctx, subject, input)
	}
	if err != nil {
		h.uploadFailure(c, err)
		return
	}
	c.JSON(200, success("上传成功", buildUploadData(view)))
}

// preupload resolves the caller's limits: guest group rules for anonymous
// uploads, or the authenticated user's group otherwise. A zero policyID
// selects the group's default rule.
func (h *Handler) preupload(ctx context.Context, c *gin.Context, guest bool, policyID uint64) (service.TokenSubject, uint64, service.UploadLimits, error) {
	if guest {
		enabled, _, ok, err := h.lsky.GuestUploadState(ctx)
		if err != nil {
			return service.TokenSubject{}, 0, service.UploadLimits{}, err
		}
		if !enabled || !ok {
			return service.TokenSubject{}, 0, service.UploadLimits{}, service.ErrUnauthenticated
		}
		limits, err := h.images.GuestPreflight(ctx, policyID)
		return service.TokenSubject{}, 0, limits, err
	}
	id := currentIdentity(c)
	if id == nil {
		return service.TokenSubject{}, 0, service.UploadLimits{}, service.ErrUnauthenticated
	}
	limits, err := h.images.Preflight(ctx, id.Subject, policyID)
	return id.Subject, id.User.ID, limits, err
}

// checkUploadFields validates size, rate limit and album ownership.
func (h *Handler) checkUploadFields(ctx context.Context, guest bool, userID uint64, ip string, fields uploadFields, limits service.UploadLimits) error {
	if guest {
		if !h.uploads.Allow("upload:guest:"+ratelimit.ClientKey(ip), limits.PerMinute) {
			return errRateLimited
		}
		if fields.albumID != 0 {
			return errGuestAlbum
		}
	} else {
		if !h.uploads.Allow(ratelimit.UploadKey(userID), limits.PerMinute) {
			return errRateLimited
		}
		if fields.albumID != 0 {
			if _, err := h.albums.FindOwned(ctx, userID, fields.albumID); err != nil {
				return errAlbumUnavailable
			}
		}
	}
	return nil
}

// uploadFailure maps every upload failure to the v1 failure envelope. Only
// authentication and throttling use semantic HTTP statuses.
func (h *Handler) uploadFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errV1MissingFile):
		c.JSON(200, failure("请选择要上传的文件"))
	case errors.Is(err, errV1MultipleFiles):
		c.JSON(200, failure("一次只能上传一个文件"))
	case errors.Is(err, errV1InvalidField):
		c.JSON(200, failure("上传参数无效"))
	case errors.Is(err, errV1TooLarge):
		c.JSON(200, failure("文件超出大小限制"))
	case errors.Is(err, errRateLimited):
		c.JSON(429, failure("Too Many Attempts."))
	case errors.Is(err, errGuestAlbum):
		c.JSON(200, failure("游客上传不支持相册"))
	case errors.Is(err, errAlbumUnavailable):
		c.JSON(200, failure("相册不存在或不可用"))
	case errors.Is(err, service.ErrUnauthenticated):
		c.JSON(401, failure("Unauthenticated."))
	case errors.Is(err, service.ErrQuotaExceeded):
		c.JSON(200, failure("容量不足，无法上传"))
	case errors.Is(err, service.ErrUnsupportedFormat):
		c.JSON(200, failure("该文件类型不被允许上传"))
	case errors.Is(err, service.ErrForbidden):
		c.JSON(200, failure("所选上传策略不可用"))
	case errors.Is(err, service.ErrPathConflict):
		c.JSON(200, failure("存储路径冲突，请重试"))
	case errors.Is(err, service.ErrImageBusy):
		c.JSON(200, failure("相同的上传正在进行中，请稍后再试"))
	case errors.Is(err, service.ErrProcessing):
		c.JSON(200, failure("图片处理失败，请稍后再试"))
	case errors.Is(err, service.ErrStorage):
		c.JSON(200, failure("存储服务异常，请稍后再试"))
	case isCanceled(err):
		c.JSON(200, failure("请求超时或已取消"))
	default:
		c.JSON(200, failure("上传失败，请稍后再试"))
	}
}

// readV1Upload drains one bounded multipart body and extracts the single
// `file` part plus the optional strategy_id, album_id and permission fields.
// Unknown fields are ignored for client compatibility.
func readV1Upload(ctx context.Context, reader *multipart.Reader, fileLimit, requestLimit int64) (uploadFile, uploadFields, error) {
	fields := uploadFields{public: false}
	var file uploadFile
	seenFile := false
	for {
		if err := ctx.Err(); err != nil {
			return file, fields, err
		}
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return file, fields, uploadReadError(ctx, err)
		}
		switch name := part.FormName(); name {
		case "file":
			if seenFile {
				_ = part.Close()
				return file, fields, errV1MultipleFiles
			}
			seenFile = true
			file, err = readV1File(ctx, part, fileLimit, requestLimit)
			_ = part.Close()
			if err != nil {
				return file, fields, err
			}
		case "strategy_id", "album_id", "permission":
			value, fieldErr := readV1Field(ctx, part)
			_ = part.Close()
			if fieldErr != nil {
				return file, fields, fieldErr
			}
			if name == "permission" {
				switch value {
				case "":
				case "1":
					fields.public = true
				case "0":
				default:
					return file, fields, errV1InvalidField
				}
				continue
			}
			parsed, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr != nil {
				return file, fields, errV1InvalidField
			}
			if name == "strategy_id" {
				fields.strategyID = parsed
			} else {
				fields.albumID = parsed
			}
		default:
			if _, err := io.Copy(io.Discard, io.LimitReader(part, 1<<20)); err != nil {
				_ = part.Close()
				return file, fields, uploadReadError(ctx, err)
			}
			_ = part.Close()
		}
	}
	if !seenFile {
		return file, fields, errV1MissingFile
	}
	return file, fields, nil
}

func readV1File(ctx context.Context, part *multipart.Part, fileLimit, requestLimit int64) (uploadFile, error) {
	limit := min(fileLimit, requestLimit)
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil {
		return uploadFile{}, uploadReadError(ctx, err)
	}
	result := uploadFile{name: part.FileName(), data: data}
	if result.name == "" {
		result.err = errV1MissingFile
		result.data = nil
	}
	if int64(len(data)) > limit {
		result.err = errV1TooLarge
		result.data = nil
	}
	return result, nil
}

func readV1Field(ctx context.Context, part *multipart.Part) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, 1025))
	if err != nil {
		return "", uploadReadError(ctx, err)
	}
	if len(data) > 1024 {
		return "", errV1InvalidField
	}
	return string(data), nil
}

func uploadReadError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errV1TooLarge
	}
	return errV1InvalidField
}
