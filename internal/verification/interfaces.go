package verification

import (
	"context"
)

type VerificationStrategy interface {
	Name() string
	Verify(ctx context.Context, path string) (bool, string, error)
}
