package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	DefaultLimit int32 = 50
	MaxLimit     int32 = 200
)

var ErrInvalidCursor = errors.New("invalid cursor")

// Request is the parsed pagination query.
type Request struct {
	Limit  int32
	Cursor string
}

// ParseRequest reads ?limit= and ?cursor=. Invalid limits are rejected rather
// than silently clamped so clients notice mistakes.
func ParseRequest(r *http.Request) (Request, error) {
	limit := DefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || int32(value) > MaxLimit {
			return Request{}, fmt.Errorf("limit must be between 1 and %d", MaxLimit)
		}
		limit = int32(value)
	}
	cursor := r.URL.Query().Get("cursor")
	if len(cursor) > 2048 {
		return Request{}, fmt.Errorf("cursor is too long")
	}
	return Request{Limit: limit, Cursor: cursor}, nil
}

type cursorPayload struct {
	Kind string    `json:"k"`
	Time time.Time `json:"t,omitempty"`
	Name string    `json:"n,omitempty"`
	ID   string    `json:"i"`
}

// TimeCursor encodes a keyset cursor for a (timestamp, id) sort.
func TimeCursor(kind string, at time.Time, id pgtype.UUID) string {
	raw, _ := json.Marshal(cursorPayload{
		Kind: kind,
		Time: at.UTC(),
		ID:   pgutil.UUIDToString(id),
	})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// NameCursor encodes a keyset cursor for a (name, id) sort.
func NameCursor(kind, name string, id pgtype.UUID) string {
	raw, _ := json.Marshal(cursorPayload{
		Kind: kind,
		Name: name,
		ID:   pgutil.UUIDToString(id),
	})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decode(raw, kind string) (cursorPayload, error) {
	if raw == "" {
		return cursorPayload{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursorPayload{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return cursorPayload{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if payload.Kind != kind {
		return cursorPayload{}, fmt.Errorf("%w: cursor belongs to %q", ErrInvalidCursor, payload.Kind)
	}
	if payload.ID == "" {
		return cursorPayload{}, fmt.Errorf("%w: missing id", ErrInvalidCursor)
	}
	return payload, nil
}

// ParseTimeCursor decodes a timestamp cursor. has is false when raw is empty.
func ParseTimeCursor(raw, kind string) (at time.Time, id pgtype.UUID, has bool, err error) {
	payload, err := decode(raw, kind)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, false, err
	}
	if raw == "" {
		return time.Time{}, pgtype.UUID{}, false, nil
	}
	if payload.Time.IsZero() {
		return time.Time{}, pgtype.UUID{}, false, fmt.Errorf("%w: missing timestamp", ErrInvalidCursor)
	}
	id, err = pgutil.ParseUUID(payload.ID)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, false, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return payload.Time, id, true, nil
}

// ParseNameCursor decodes a name cursor. has is false when raw is empty.
func ParseNameCursor(raw, kind string) (name string, id pgtype.UUID, has bool, err error) {
	payload, err := decode(raw, kind)
	if err != nil {
		return "", pgtype.UUID{}, false, err
	}
	if raw == "" {
		return "", pgtype.UUID{}, false, nil
	}
	id, err = pgutil.ParseUUID(payload.ID)
	if err != nil {
		return "", pgtype.UUID{}, false, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	return payload.Name, id, true, nil
}

// SetNextLink sets an RFC 8288 Link header for the next page.
func SetNextLink(w http.ResponseWriter, r *http.Request, next string) {
	if next == "" {
		return
	}
	u := *r.URL
	q := u.Query()
	q.Set("cursor", next)
	u.RawQuery = q.Encode()
	w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", u.String()))
}

// TrimPage returns the requested page and whether more rows exist. Callers
// should fetch limit+1 rows.
func TrimPage[T any](items []T, limit int32) ([]T, bool) {
	if int32(len(items)) <= limit {
		return items, false
	}
	return items[:limit], true
}
