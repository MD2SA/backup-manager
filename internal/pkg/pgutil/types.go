package pgutil

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func ToText(s string) pgtype.Text {
	return pgtype.Text{
		String: s,
		Valid:  s != "",
	}
}

func ToTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  t,
		Valid: !t.IsZero(),
	}
}

func ToTimestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return ToTimestamptz(*t)
}

func ToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}

func ToIntervalPtr(d *time.Duration) pgtype.Interval {
	if d == nil {
		return pgtype.Interval{Valid: false}
	}
	return ToInterval(*d)
}

func ToInt8(i int64) pgtype.Int8 {
	return pgtype.Int8{
		Int64: i,
		Valid: true,
	}
}

func ToInt8Ptr(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return ToInt8(*i)
}

func IntervalToString(i pgtype.Interval) string {
	if !i.Valid {
		return ""
	}
	return (time.Duration(i.Microseconds) * time.Microsecond).String()
}
