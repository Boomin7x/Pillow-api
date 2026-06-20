package notify

import (
	"context"
	"log/slog"

	"github.com/kodiahbertrand/pillow/internal/domain"
)

type logNotifier struct{}

func New() *logNotifier {
	return &logNotifier{}
}

func (n *logNotifier) Notify(_ context.Context, notif domain.Notification) error {
	slog.Info("kyc: notification",
		"user_id", notif.UserID,
		"event_type", notif.EventType,
		"case_id", notif.CaseID,
		"status", string(notif.Status),
	)
	return nil
}
