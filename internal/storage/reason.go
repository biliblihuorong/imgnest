package storage

import (
	"crypto/x509"
	"errors"
	"net"
	"os"
	"syscall"

	"github.com/aws/smithy-go"
)

// Reason classifies a driver failure into a short, stable hint code that a
// client can explain to an administrator, such as "storage_denied" for
// rejected credentials or "storage_disk_full" for a full local disk. It
// never returns provider messages, keys or endpoints, and returns "" when
// the cause is unknown.
func Reason(err error) string {
	if err == nil {
		return ""
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken",
			"InvalidToken", "AllAccessDisabled", "AuthorizationHeaderMalformed", "InvalidSecurity",
			"Unauthorized", "Forbidden":
			return "storage_denied"
		case "NoSuchBucket", "InvalidBucketName", "PermanentRedirect":
			return "storage_bucket"
		case "RequestTimeTooSkewed":
			return "storage_clock"
		case "SlowDown", "ServiceUnavailable", "InternalError", "RequestTimeout", "TooManyRequests":
			return "storage_busy"
		case "QuotaExceeded", "StorageQuotaExceeded", "InsufficientStorage":
			return "storage_quota"
		}
	}
	if errors.Is(err, ErrUnsupported) {
		return "storage_unsupported"
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid) {
		return "storage_tls"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "storage_unreachable"
	}
	switch {
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return "storage_disk_full"
	case errors.Is(err, syscall.EROFS):
		return "storage_read_only"
	case errors.Is(err, os.ErrPermission):
		return "storage_permission"
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "storage_unreachable"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "storage_timeout"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "storage_unreachable"
	}
	return ""
}
