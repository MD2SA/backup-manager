package notification

import (
	"context"
)

type Event string

const (
	EventStarted Event = "started"
	EventSuccess Event = "success"
	EventFailed  Event = "failed"
	EventOverdue Event = "overdue"
)

type NotificationProvider interface {
	Name() string
	Send(ctx context.Context, event Event, message string, metadata map[string]interface{}) error
}
