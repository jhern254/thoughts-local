package subject

import (
	"errors"

	"github.com/jhern254/go-thoughts/internal/data"
)

// IsExpectedError reports outcomes that delivery layers should not log as failures.
func IsExpectedError(err error) bool {
	var validationError *ValidationError
	return errors.As(err, &validationError) ||
		errors.Is(err, data.ErrRecordNotFound) ||
		errors.Is(err, data.ErrDuplicateRecord)
}
