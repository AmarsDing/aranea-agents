package artifact

import "errors"

// MaxUploadBytes is the maximum artifact payload size (10 MB).
// Larger files are not supported; streaming upload is planned separately.
const MaxUploadBytes = 10 << 20

// ErrSizeExceeded is returned when payload exceeds MaxUploadBytes.
var ErrSizeExceeded = errors.New("artifact exceeds 10 MB size limit")

// ValidateUploadSize rejects payloads above MaxUploadBytes.
func ValidateUploadSize(n int64) error {
	if n > MaxUploadBytes {
		return ErrSizeExceeded
	}
	return nil
}
