package handlers

import (
	"net/http"

	"github.com/MD2SA/backup-manager/internal/api/dto"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/version"
)

// Version
// @Summary Get build information
// @Description Public build version, commit, date and Go runtime.
// @Tags health
// @Produce json
// @Success 200 {object} dto.VersionResponse
// @Router /version [get]
func Version(w http.ResponseWriter, r *http.Request) {
	apiutil.Success(w, http.StatusOK, dto.VersionResponse{
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
		Go:      version.GoVersion(),
	})
}
