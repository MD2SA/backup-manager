package pgutil

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func ParseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if s == "" {
		return u, nil
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return u, fmt.Errorf("invalid uuid: %w", err)
	}
	u.Bytes = parsed
	u.Valid = true
	return u, nil
}

func NewUUID() pgtype.UUID {
	return pgtype.UUID{
		Bytes: uuid.New(),
		Valid: true,
	}
}

func UUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}
