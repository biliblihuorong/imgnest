// Package service implements ImgNest business rules through persistence interfaces.
package service

import "github.com/biliblihuorong/imgnest/internal/model"

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
