package artifact

import kerrors "github.com/go-kratos/kratos/v2/errors"

// MaxUploadBytes is the maximum artifact payload size (10 MB).
// Larger files are not supported; streaming upload is planned separately.
const MaxUploadBytes = 10 << 20

// ErrSizeExceeded is returned when payload exceeds MaxUploadBytes.
var ErrSizeExceeded = kerrors.BadRequest("ARTIFACT", "artifact exceeds 10 MB size limit")

// ValidateUploadSize rejects payloads above MaxUploadBytes.
func ValidateUploadSize(n int64) error {
	if n > MaxUploadBytes {
		return ErrSizeExceeded
	}
	return nil
}
