package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// Principal is the authenticated identity attached to a request.
type Principal struct {
	UserID    pgtype.UUID
	Email     string
	Name      string
	KeyID     pgtype.UUID
	KeyPrefix string
}

func (p Principal) Insecure() bool {
	return !p.UserID.Valid
}

type principalContextKey struct{}

// WithPrincipal stores the authenticated identity in a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated identity, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
