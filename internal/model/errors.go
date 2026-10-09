package model

import "errors"

// Shared domain errors keep repository implementations independent of services.
var (
	ErrInvalidInput         = errors.New("invalid input")
	ErrNotFound             = errors.New("not found")
	ErrRegistrationDisabled = errors.New("registration disabled")
	ErrUserExists           = errors.New("user already exists")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrForbidden            = errors.New("forbidden")
	ErrUnauthenticated      = errors.New("unauthenticated")
	ErrQuotaExceeded        = errors.New("quota exceeded")
	ErrPathConflict         = errors.New("path conflict")
	ErrImageBusy            = errors.New("image operation is busy")
	ErrUnsupportedFormat    = errors.New("unsupported image format")
	ErrStorage              = errors.New("storage operation failed")
	ErrProcessing           = errors.New("image processing failed")
	// ErrGroupHasMembers rejects deleting a group that still owns accounts.
	ErrGroupHasMembers = errors.New("group still has members")
	// ErrStillReferenced rejects deleting a resource another record points at,
	// such as a storage backend referenced by a rule.
	ErrStillReferenced = errors.New("resource is still referenced")
	// ErrRandomLinkExists reports a random link whose album or token is taken.
	ErrRandomLinkExists = errors.New("random link exists")
	// ErrIdentityNotLinked reports an external sign-in whose subject has no
	// local account and policy allows neither linking nor creating one.
	ErrIdentityNotLinked = errors.New("external identity is not linked")
	// ErrIdentityEmailRequired reports an external sign-in that would create
	// an account but carries no verified email address.
	ErrIdentityEmailRequired = errors.New("external identity has no verified email")
)
