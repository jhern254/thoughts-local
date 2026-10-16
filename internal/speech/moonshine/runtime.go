package moonshine

import "context"

// CheckRuntime checks the linked, pinned C API without opening a model. Ordinary
// builds return the fixed unavailable-runtime category and require no libraries.
func CheckRuntime(ctx context.Context) error {
	if err := acquireMoonshineRuntime(ctx); err != nil {
		return err
	}
	defer releaseMoonshineRuntime()
	if err := ctx.Err(); err != nil {
		return err
	}
	err := checkNativeRuntime()
	if lifecycleErr := ctx.Err(); lifecycleErr != nil {
		return lifecycleErr
	}
	return err
}
