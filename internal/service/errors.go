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
)
