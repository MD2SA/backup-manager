package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MD2SA/backup-manager/internal/api/dto"
	"github.com/MD2SA/backup-manager/internal/version"
)

func TestVersionHandler(t *testing.T) {
	version.Version = "test-version"
	version.Commit = "test-commit"
	version.Date = "test-date"

	rec := httptest.NewRecorder()
	Version(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var response dto.VersionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Version != "test-version" || response.Commit != "test-commit" || response.Date != "test-date" {
		t.Fatalf("unexpected version response: %+v", response)
	}
	if response.Go == "" {
		t.Fatal("go version is empty")
	}
}
