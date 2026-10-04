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
)
