// Package extension is the stable surface that editions built outside this
// repository (such as ImgNest Pro) use to add features to the server. Packages
// under internal/ stay private; everything an extension may rely on is here.
package extension

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginProvider is an extra sign-in option the login page offers next to the
// password form, such as "Sign in with GitHub".
type LoginProvider struct {
	// ID is a short stable key, unique among the providers of one plugin.
	ID string `json:"id"`
	// Name is the button label shown to visitors.
	Name string `json:"name"`
	// StartURL is the same-origin path the button navigates to; the plugin's
	// own route there starts the sign-in flow.
	StartURL string `json:"start_url"`
}

// Plugin is one server extension. Plugins are passed to app.Execute and are
// mounted when the serve command builds its router.
type Plugin interface {
	// Name is a lowercase identifier ([a-z0-9-], 1 to 32 characters); the
	// plugin's routes live under /api/ext/{Name}.
	Name() string
	// Mount registers the plugin's routes on its own /api/ext/{Name} group, so
	// a plugin can never shadow a core route. host finishes sign-ins.
	Mount(ctx context.Context, router gin.IRouter, host Host) error
	// LoginProviders lists the sign-in options to show on the login page. It
	// is called for every public site read and must be cheap.
	LoginProviders(ctx context.Context) []LoginProvider
}

// ExternalIdentity is a user the plugin has authenticated with an external
// provider, for example from a verified OIDC ID token.
type ExternalIdentity struct {
	// Provider is the plugin's provider ID ("github"); the host namespaces it
	// with the plugin name before storing it.
	Provider string
	// Subject is the provider's stable, never-reassigned user ID. Never use an
	// email address or a renameable login here.
	Subject string
	Email   string
	// EmailVerified is true only when the provider asserts the address.
	EmailVerified bool
	// Username seeds the username of an account created on first sign-in.
	Username string
	// LinkByEmail lets a first-time subject attach to an existing
	// non-administrator account with the same verified email. Enable it only
	// for providers whose email claims you trust.
	LinkByEmail bool
}

// Host is what ImgNest offers a mounted plugin.
type Host interface {
	// CompleteSignIn signs the identity in (linking it, or creating an
	// account when registration is open) and redirects the browser back to
	// the web app, which finishes the login. It always writes the response.
	CompleteSignIn(c *gin.Context, identity ExternalIdentity)
	// FailSignIn redirects the browser to the login page with a generic
	// error, for flows the plugin itself rejected (bad state, denied consent).
	FailSignIn(c *gin.Context)
}

// The interfaces below are optional: a Plugin that also implements one of them
// is wired into that part of the server. A plugin that implements none of them
// changes nothing outside its own routes.

// Event types delivered to EventSubscriber.
const (
	EventImageUploaded  = "image.uploaded"
	EventImageTrashed   = "image.trashed"
	EventImageRestored  = "image.restored"
	EventImagePurged    = "image.purged"
	EventUserRegistered = "user.registered"
)

// Event is a change the server has already committed. Image or User is set
// according to Type. Events never carry EXIF, credentials or email addresses.
type Event struct {
	Type  string
	At    time.Time
	Image *EventImage
	User  *EventUser
}

// EventImage describes the image an event is about. Links are the public
// object URLs the image had when the event happened; empty ones do not exist.
type EventImage struct {
	ID        uint64
	Key       string
	UserID    uint64
	AlbumID   uint64
	StorageID uint64
	Path      string
	Name      string
	MIME      string
	Size      int64
	Width     int
	Height    int
	IsPublic  bool
	Original  string
	WebP      string
	Thumbnail string
}

// EventUser describes the account an event is about.
type EventUser struct {
	ID       uint64
	Username string
}

// EventSubscriber receives committed events. Each subscriber gets its own
// bounded queue and goroutine, so a slow subscriber delays only itself; when
// its queue is full, further events for it are dropped and logged.
type EventSubscriber interface {
	HandleEvent(ctx context.Context, event Event)
}

// DisplayImage describes an upload whose display WebP is being produced.
type DisplayImage struct {
	UserID    uint64 // 0 for guest uploads
	GroupID   uint64
	PolicyID  uint64
	StorageID uint64
	// Format is the detected source format ("jpeg", "png", "gif"...).
	Format string
	// Width and Height are the display WebP's pixel size; Frames is above 1
	// for animations.
	Width, Height, Frames int
	// Quality, Effort and Lossless are the rule's WebP settings, for
	// re-encoding the transformed image the same way.
	Quality, Effort int
	Lossless        bool
}

// DisplayTransformer may rewrite the WebP display copy an upload produces,
// for example to add a watermark. It is called only when the server encoded a
// separate WebP from the source: the stored original, thumbnails and uploads
// that already were WebP are never passed in, so the original is never
// re-encoded. The result must be a WebP with the same size and frame count;
// anything else, or an error, rejects the upload. Return the input unchanged
// to leave an image alone.
type DisplayTransformer interface {
	TransformDisplay(ctx context.Context, image DisplayImage, webp []byte) ([]byte, error)
}

// Access kinds passed to AccessGuard.
const (
	// AccessObject is a direct link the server itself serves (/i/...), which
	// exists only for local storage; S3 links never reach the server.
	AccessObject = "object"
	// AccessThumbnail is a local preview (/t/...).
	AccessThumbnail = "thumbnail"
	// AccessRandom is a random-image link (/random/...), for every storage.
	AccessRandom = "random"
)

// AccessGuard can refuse public image requests the server answers itself,
// for example by Referer. It runs before the core handler; returning false
// means the guard has already written the response.
type AccessGuard interface {
	GuardAccess(c *gin.Context, kind string) bool
}
