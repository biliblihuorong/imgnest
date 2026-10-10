package service

import (
	"context"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Event types published to the EventSink.
const (
	EventImageUploaded  = "image.uploaded"
	EventImageTrashed   = "image.trashed"
	EventImageRestored  = "image.restored"
	EventImagePurged    = "image.purged"
	EventUserRegistered = "user.registered"
)

// EventSink receives changes after they are committed. Publish must not block
// the caller; delivery is best effort.
type EventSink interface {
	Publish(ctx context.Context, event Event)
}

// Event is one committed change. It never carries EXIF, credentials or email.
type Event struct {
	Type  string
	At    time.Time
	Image *EventImage
	User  *EventUser
}

// EventImage is the public description of an image in an event.
type EventImage struct {
	ID, UserID, AlbumID, StorageID uint64
	Key, Path, Name, MIME          string
	Size                           int64
	Width, Height                  int
	IsPublic                       bool
	Original, WebP, Thumbnail      string
}

// EventUser is the public description of an account in an event.
type EventUser struct {
	ID       uint64
	Username string
}

// DisplayImage describes the upload whose display WebP is being transformed.
type DisplayImage struct {
	UserID, GroupID, PolicyID, StorageID uint64
	Format                               string
	Width, Height, Frames                int
	Quality, Effort                      int
	Lossless                             bool
}

// TrashPolicy may override the recycle-bin retention of one owner's image;
// ok=false keeps the site setting.
type TrashPolicy interface {
	TrashDays(ctx context.Context, userID, groupID uint64) (days int, ok bool)
}

// DisplayTransformer may rewrite an upload's separately encoded display WebP.
type DisplayTransformer interface {
	TransformDisplay(ctx context.Context, image DisplayImage, webp []byte) ([]byte, error)
}

// UploadInspection describes an upload an UploadInspector checks before
// anything is stored.
type UploadInspection struct {
	UserID, GroupID, PolicyID, StorageID uint64
	Filename, Format                     string
	Width, Height, Frames                int
	Size                                 int64
}

// UploadInspector may refuse an upload with ErrContentRejected or
// ErrReviewUnavailable; any other error fails the upload as a processing
// error. image is the display copy (the encoded WebP, else the original).
type UploadInspector interface {
	InspectUpload(ctx context.Context, upload UploadInspection, image []byte) error
}

func (s *ImageService) emit(ctx context.Context, kind string, image model.Image, backend model.Storage) {
	if s.deps.Events == nil {
		return
	}
	value := &EventImage{ID: image.ID, UserID: image.UserID, AlbumID: image.AlbumID, StorageID: image.StorageID, Key: image.Key, Path: image.Path, Name: image.OriginName, MIME: image.MIME, Size: image.Size, Width: image.Width, Height: image.Height, IsPublic: image.IsPublic}
	if image.HasOriginal {
		value.Original = objectURL(backend.BaseURL, image.Path+"."+image.Ext)
	}
	if image.HasWebP {
		value.WebP = objectURL(backend.BaseURL, image.Path+".webp")
	}
	if image.HasThumb {
		value.Thumbnail = objectURL(backend.BaseURL, image.Path+"_thumbs.webp")
	}
	s.deps.Events.Publish(ctx, Event{Type: kind, At: s.deps.Now().UTC(), Image: value})
}

func (s *UserService) emitRegistered(ctx context.Context, user model.User) {
	if s.events == nil {
		return
	}
	s.events.Publish(ctx, Event{Type: EventUserRegistered, At: time.Now().UTC(), User: &EventUser{ID: user.ID, Username: user.Username}})
}

// UseEvents publishes account events to sink; nil disables them.
func (s *UserService) UseEvents(sink EventSink) { s.events = sink }
