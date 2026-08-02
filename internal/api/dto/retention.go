package dto

import (
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/pkg/validator"
	"github.com/MD2SA/backup-manager/internal/repository/db"
)

type RetentionPolicyRequest struct {
	Name        string `json:"name" validate:"required"`
	KeepHourly  int32  `json:"keep_hourly" validate:"min=0"`
	KeepDaily   int32  `json:"keep_daily" validate:"min=0"`
	KeepWeekly  int32  `json:"keep_weekly" validate:"min=0"`
	KeepMonthly int32  `json:"keep_monthly" validate:"min=0"`
	KeepYearly  int32  `json:"keep_yearly" validate:"min=0"`
	YearlyMonth int32  `json:"yearly_month" validate:"min=1,max=12"`
}

func (r *RetentionPolicyRequest) Validate() error {
	return validator.Get().Struct(r)
}

type RetentionPolicyResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	KeepHourly  int32     `json:"keep_hourly"`
	KeepDaily   int32     `json:"keep_daily"`
	KeepWeekly  int32     `json:"keep_weekly"`
	KeepMonthly int32     `json:"keep_monthly"`
	KeepYearly  int32     `json:"keep_yearly"`
	YearlyMonth int32     `json:"yearly_month"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func ToRetentionPolicyResponse(p db.RetentionPolicy) RetentionPolicyResponse {
	return RetentionPolicyResponse{
		ID:          pgutil.UUIDToString(p.ID),
		Name:        p.Name,
		KeepHourly:  p.KeepHourly,
		KeepDaily:   p.KeepDaily,
		KeepWeekly:  p.KeepWeekly,
		KeepMonthly: p.KeepMonthly,
		KeepYearly:  p.KeepYearly,
		YearlyMonth: p.YearlyMonth,
		CreatedAt:   p.CreatedAt.Time,
		UpdatedAt:   p.UpdatedAt.Time,
	}
}
