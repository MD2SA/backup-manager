package pagination

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
)

func TestTimeCursorRoundtrip(t *testing.T) {
	at := time.Date(2026, 11, 11, 12, 0, 0, 123456000, time.UTC)
	id := pgutil.NewUUID()
	raw := TimeCursor("executions", at, id)
	if raw == "" {
		t.Fatal("empty cursor")
	}

	gotAt, gotID, has, err := ParseTimeCursor(raw, "executions")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !has || !gotAt.Equal(at) || gotID != id {
		t.Fatalf("unexpected cursor: %v %v %v", has, gotAt, gotID)
	}
}

func TestTimeCursorWrongKind(t *testing.T) {
	raw := TimeCursor("executions", time.Now(), pgutil.NewUUID())
	if _, _, _, err := ParseTimeCursor(raw, "restores"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestNameCursorRoundtrip(t *testing.T) {
	id := pgutil.NewUUID()
	raw := NameCursor("profiles", "daily", id)
	name, gotID, has, err := ParseNameCursor(raw, "profiles")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !has || name != "daily" || gotID != id {
		t.Fatalf("unexpected cursor: %v %q %v", has, name, gotID)
	}
}

func TestCursorInvalidBase64(t *testing.T) {
	if _, _, _, err := ParseTimeCursor("not-base64!!", "executions"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestParseRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/executions?limit=25", nil)
	page, err := ParseRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if page.Limit != 25 {
		t.Fatalf("limit = %d", page.Limit)
	}

	req = httptest.NewRequest("GET", "/executions", nil)
	page, err = ParseRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if page.Limit != DefaultLimit {
		t.Fatalf("default limit = %d", page.Limit)
	}

	for _, query := range []string{"?limit=0", "?limit=-1", "?limit=201", "?limit=abc"} {
		req = httptest.NewRequest("GET", "/executions"+query, nil)
		if _, err := ParseRequest(req); err == nil {
			t.Errorf("expected error for %s", query)
		}
	}
}

func TestTrimPage(t *testing.T) {
	items, more := TrimPage([]int{1, 2, 3}, 2)
	if !more || len(items) != 2 {
		t.Fatalf("unexpected trim: %v %v", items, more)
	}
	items, more = TrimPage([]int{1}, 2)
	if more || len(items) != 1 {
		t.Fatalf("unexpected trim: %v %v", items, more)
	}
}

func TestSetNextLink(t *testing.T) {
	req := httptest.NewRequest("GET", "/executions?limit=25", nil)
	rec := httptest.NewRecorder()
	SetNextLink(rec, req, "abc")
	if got := rec.Header().Get("Link"); got != `</executions?cursor=abc&limit=25>; rel="next"` {
		t.Fatalf("link = %q", got)
	}
}
