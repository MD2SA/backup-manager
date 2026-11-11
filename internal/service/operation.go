package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// operationContext bounds the resource-heavy part of a backup or restore while
// leaving the parent context available for status updates and notifications.
// A zero timeout disables the limit.
func operationContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// operationError adds a clear label when the operation exceeded its timeout.
func operationError(label string, timeout time.Duration, opCtx context.Context, err error) error {
	if timeout > 0 && errors.Is(opCtx.Err(), context.DeadlineExceeded) {
		if err == nil {
			return fmt.Errorf("%s timed out after %s", label, timeout)
		}
		return fmt.Errorf("%s timed out after %s: %w", label, timeout, err)
	}
	return err
}
