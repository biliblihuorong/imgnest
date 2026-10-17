// Package service implements ImgNest business rules through persistence interfaces.
package service

import (
	"errors"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// Domain errors are shared with persistence without a dependency on this package.
var (
	// ErrInvalidInput identifies rejected request values.
	ErrInvalidInput = model.ErrInvalidInput
	// ErrNotFound identifies a missing persistence record.
	ErrNotFound = model.ErrNotFound
	// ErrRegistrationDisabled identifies the site's closed registration policy.
	ErrRegistrationDisabled = model.ErrRegistrationDisabled
	// ErrUserExists identifies a conflicting username or email.
	ErrUserExists = model.ErrUserExists
	// ErrInvalidCredentials conceals whether an account or password failed verification.
	ErrInvalidCredentials = model.ErrInvalidCredentials
	// ErrForbidden identifies an operation outside the user's permissions.
	ErrForbidden = model.ErrForbidden
	// ErrUnauthenticated identifies a missing, invalid, expired, or revoked credential.
	ErrUnauthenticated = model.ErrUnauthenticated
	// ErrQuotaExceeded identifies insufficient actual cloud-object capacity.
	ErrQuotaExceeded = model.ErrQuotaExceeded
	// ErrPathConflict identifies an occupied image path or object key.
	ErrPathConflict = model.ErrPathConflict
	// ErrImageBusy identifies an unfinished storage transition.
	ErrImageBusy = model.ErrImageBusy
	// ErrUnsupportedFormat identifies a disallowed image format.
	ErrUnsupportedFormat = model.ErrUnsupportedFormat
	// ErrStorage identifies a failed storage operation without provider details.
	ErrStorage = model.ErrStorage
	// ErrProcessing identifies rejected image processing or metadata sanitization.
	ErrProcessing = model.ErrProcessing
	// ErrGroupHasMembers rejects deleting a group that still owns accounts.
	ErrGroupHasMembers = model.ErrGroupHasMembers
	// ErrStillReferenced rejects deleting a storage, policy, or group that
	// other records still point at.
	ErrStillReferenced = model.ErrStillReferenced
	// ErrIdentityNotLinked rejects an external sign-in with no usable account.
	ErrIdentityNotLinked = model.ErrIdentityNotLinked
	// ErrIdentityEmailRequired rejects creating an account from an external
	// sign-in that carries no verified email.
	ErrIdentityEmailRequired = model.ErrIdentityEmailRequired
	// ErrContentRejected rejects an upload a content review refused.
	ErrContentRejected = model.ErrContentRejected
	// ErrReviewUnavailable rejects an upload whose content review could not run.
	ErrReviewUnavailable = model.ErrReviewUnavailable
	// ErrUploadLimitReached rejects an upload over a plugin's upload allowance.
	ErrUploadLimitReached = model.ErrUploadLimitReached
)

// ProcessingStep names the image pipeline step a ProcessingError stopped at.
type ProcessingStep string

// Pipeline steps reported by ProcessingError.
const (
	ProcessingDecode   ProcessingStep = "decode"
	ProcessingPixels   ProcessingStep = "pixels"
	ProcessingMetadata ProcessingStep = "metadata"
	ProcessingScrub    ProcessingStep = "scrub"
	ProcessingConvert  ProcessingStep = "convert"
)

// ProcessingError is ErrProcessing with the step that failed, so an operator
// report can say why; clients still only see ErrProcessing.
type ProcessingError struct {
	Step ProcessingStep
}

func (e *ProcessingError) Error() string { return "image processing failed at " + string(e.Step) }

// Is makes a ProcessingError match ErrProcessing.
func (e *ProcessingError) Is(target error) bool { return target == ErrProcessing }

// ReasonError narrows a domain error with a short, stable reason code that
// clients can turn into a precise hint ("the password must be 12-72
// characters"). Reasons are fixed identifiers and never carry input values,
// credentials or provider messages.
type ReasonError struct {
	Reason string
	Err    error
}

func (e *ReasonError) Error() string { return e.Err.Error() + " (" + e.Reason + ")" }

// Unwrap keeps errors.Is matching the underlying domain error.
func (e *ReasonError) Unwrap() error { return e.Err }

// Input reasons for the account and image forms.
var (
	errPasswordLength     = &ReasonError{Reason: "password_length", Err: ErrInvalidInput}
	errUsernameLength     = &ReasonError{Reason: "username_length", Err: ErrInvalidInput}
	errEmailInvalid       = &ReasonError{Reason: "email_invalid", Err: ErrInvalidInput}
	errDisplayNameInvalid = &ReasonError{Reason: "display_name_invalid", Err: ErrInvalidInput}
	errImageNameInvalid   = &ReasonError{Reason: "image_name_invalid", Err: ErrInvalidInput}
)

// errScrubTooLarge rejects a file over the metadata size limit whose
// original must be scrubbed and cannot be replaced by its WebP alone.
var errScrubTooLarge = &ReasonError{Reason: "scrub_too_large", Err: &ProcessingError{Step: ProcessingScrub}}

// ErrorReason returns the hint code carried by err: an explicit ReasonError,
// or for storage failures the class of the underlying driver error (denied
// credentials, a missing bucket, an unreachable endpoint, a full disk...).
// It returns "" when there is nothing more specific to say.
func ErrorReason(err error) string {
	var reasoned *ReasonError
	if errors.As(err, &reasoned) {
		return reasoned.Reason
	}
	if errors.Is(err, ErrStorage) {
		return storage.Reason(err)
	}
	return ""
}

// storageFailure is ErrStorage that keeps the redacted driver error so
// ErrorReason can classify it; its message stays the generic one.
type storageFailure struct{ cause error }

func (e storageFailure) Error() string        { return ErrStorage.Error() }
func (e storageFailure) Unwrap() error        { return e.cause }
func (e storageFailure) Is(target error) bool { return target == ErrStorage }
