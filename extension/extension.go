// Package extension is the stable surface that editions built outside this
// repository (such as ImgNest Pro) use to add features to the server. Packages
// under internal/ stay private; everything an extension may rely on is here.
package extension

import (
	"context"
	"encoding/json"
	"errors"
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

// TrashOwner identifies whose image is being moved to the recycle bin.
type TrashOwner struct {
	UserID, GroupID uint64
}

// TrashPolicy may set how many days an image stays in the recycle bin
// before it is purged, overriding the site's setting, for example per user
// group. It is asked when an image is trashed; ok=false keeps the site
// setting. days is clamped to 0..36500, and 0 purges at once. Guest uploads
// are never passed to it.
type TrashPolicy interface {
	TrashDays(ctx context.Context, owner TrashOwner) (days int, ok bool)
}

// Counters are durable integer counters shared by every server instance,
// kept per plugin. name groups counters (for example "uploads:2026-10") and
// subject identifies one (for example a user ID); both are at most 80
// characters. Counters untouched for 400 days are deleted.
type Counters interface {
	// Add adds delta (which may be negative) and returns the new total.
	Add(ctx context.Context, name, subject string, delta int64) (int64, error)
	// Get returns a counter's total; a missing counter is 0.
	Get(ctx context.Context, name, subject string) (int64, error)
}

// CounterUser receives the plugin's own Counters at startup, before Mount.
type CounterUser interface {
	UseCounters(counters Counters)
}

// UploadImage describes an upload being inspected. Nothing has been stored
// yet when UploadInspector runs.
type UploadImage struct {
	UserID    uint64 // 0 for guest uploads
	GroupID   uint64
	PolicyID  uint64
	StorageID uint64
	// Filename is the client's file name, for logs and messages only.
	Filename string
	// Format is the detected source format ("jpeg", "png", "gif"...).
	Format string
	// Width and Height are the source's pixel size; Frames is above 1 for
	// animations. Size is the uploaded byte count.
	Width, Height, Frames int
	Size                  int64
}

// Errors an UploadInspector returns (wrapped or not) to refuse an upload.
// Any other error is reported to the uploader as a processing failure.
var (
	// ErrUploadRejected refuses content the plugin judged unacceptable.
	ErrUploadRejected = errors.New("upload rejected by content review")
	// ErrReviewUnavailable refuses an upload because the review itself
	// could not run, for plugins configured to fail closed.
	ErrReviewUnavailable = errors.New("content review unavailable")
	// ErrUploadLimitReached refuses an upload because the account used up an
	// allowance the plugin enforces, such as uploads per day.
	ErrUploadLimitReached = errors.New("upload limit reached")
)

// UploadInspector may refuse an upload after the server has decoded and
// processed it but before anything is stored or counted against quota, for
// example to send it to a content-moderation service. image is the display
// copy: the WebP the server encoded or, when it encoded none, the scrubbed
// original. Inspectors run in order and before any DisplayTransformer, so
// they see the image without watermarks. Return nil to accept.
type UploadInspector interface {
	InspectUpload(ctx context.Context, upload UploadImage, image []byte) error
}

// Setting field types for SettingField.Type.
const (
	// SettingText is a single-line string.
	SettingText = "text"
	// SettingTextarea is a multi-line string.
	SettingTextarea = "textarea"
	// SettingSecret is a string the console never shows again once saved. It
	// is stored encrypted, which requires security.master_key.
	SettingSecret = "secret"
	// SettingBool is a switch.
	SettingBool = "bool"
	// SettingInt is a whole number; SettingNumber may have a fraction.
	SettingInt    = "int"
	SettingNumber = "number"
	// SettingSelect is one value from Options or OptionsFrom; with Multiple
	// it is a list of them.
	SettingSelect = "select"
	// SettingTags is a list of free strings, such as host names.
	SettingTags = "tags"
	// SettingList is a list of objects whose fields are Fields, such as
	// webhook targets.
	SettingList = "list"
)

// Option sources for SettingField.OptionsFrom; option values are record IDs.
const (
	OptionsGroups   = "groups"
	OptionsPolicies = "policies"
	OptionsStorages = "storages"
)

// SettingOption is one choice of a select field. Value is a string or a
// number and is what the plugin receives.
type SettingOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// SettingField is one input on a plugin's settings card.
type SettingField struct {
	// Key is the field's JSON name ([a-z0-9_], unique within its level).
	Key   string `json:"key"`
	Label string `json:"label"`
	// Help is shown under the input.
	Help        string `json:"help,omitempty"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
	// Required rejects empty strings, empty lists and unset selects.
	Required bool `json:"required,omitempty"`
	// Default is the value used while nothing has been saved; it must have
	// the field's JSON shape (string, bool, number, list...).
	Default any `json:"default,omitempty"`
	// Min and Max bound int and number fields.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Options are a select field's fixed choices; OptionsFrom fills them
	// from the site's records instead.
	Options     []SettingOption `json:"options,omitempty"`
	OptionsFrom string          `json:"options_from,omitempty"`
	Multiple    bool            `json:"multiple,omitempty"`
	// Fields are the fields of each item of a list field (one level only).
	Fields []SettingField `json:"fields,omitempty"`
	// ItemLabel names the item field used as a list item's title.
	ItemLabel string `json:"item_label,omitempty"`
}

// SettingsSchema describes a plugin's card on the console's extension page.
type SettingsSchema struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Fields      []SettingField `json:"fields"`
}

// SettingsError is an invalid setting the administrator can fix. Message is
// shown to them as is, so it must not contain secrets.
type SettingsError struct {
	// Field is the field's key, or "" for the whole card.
	Field   string
	Message string
}

func (e *SettingsError) Error() string { return e.Message }

// Configurable plugins get a settings card in the console. The server stores
// the values (secret fields encrypted) and hands them to ApplySettings at
// startup and after every save; it has already checked each value's type,
// bounds, options and Required, filled defaults and dropped unknown keys.
type Configurable interface {
	// SettingsSchema returns the card. ok=false hides it for now (for
	// example while a feature is unavailable); the stored values are still
	// applied at startup.
	SettingsSchema(ctx context.Context) (schema SettingsSchema, ok bool)
	// ApplySettings receives the values as one JSON object. It must validate
	// everything before changing behavior and leave the previous settings in
	// force when it returns an error; a *SettingsError is shown to the
	// administrator and the values are not saved.
	ApplySettings(ctx context.Context, values json.RawMessage) error
}

// Setting status levels for SettingStatus.Level.
const (
	StatusInfo    = "info"
	StatusSuccess = "success"
	StatusWarning = "warning"
	StatusError   = "error"
)

// SettingStatus is a read-only line shown at the top of a settings card,
// such as a license's holder and expiry.
type SettingStatus struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Level string `json:"level"`
}

// SettingsStatusReporter adds status lines to a Configurable plugin's card;
// it is called on every console read and must be cheap.
type SettingsStatusReporter interface {
	SettingsStatus(ctx context.Context) []SettingStatus
}
