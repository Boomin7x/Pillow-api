package auth

import (
	"context"
	"log/slog"
	"time"

	"github.com/kodiahbertrand/pillow/internal/domain"
	pgmodels "github.com/kodiahbertrand/pillow/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

const (
	auditBatchSize  = 50
	auditFlushEvery = 100 * time.Millisecond
)

type auditLogger struct {
	ch   chan *domain.AuthEvent
	db   *gorm.DB
	done chan struct{}
}

func NewAuditLogger(db *gorm.DB, bufSize int) *auditLogger {
	return &auditLogger{
		ch:   make(chan *domain.AuthEvent, bufSize),
		db:   db,
		done: make(chan struct{}),
	}
}

func (l *auditLogger) Log(_ context.Context, event *domain.AuthEvent) {
	select {
	case l.ch <- event:
	default:
		slog.Error("audit logger: channel full, event dropped", "event_type", event.EventType)
	}
}

func (l *auditLogger) Run(ctx context.Context) {
	defer close(l.done)

	ticker := time.NewTicker(auditFlushEvery)
	defer ticker.Stop()

	batch := make([]*domain.AuthEvent, 0, auditBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		models := make([]pgmodels.AuthEventModel, len(batch))
		for i, e := range batch {
			models[i] = *pgmodels.AuthEventModelFrom(e)
		}
		if err := l.db.Create(&models).Error; err != nil {
			slog.Error("audit logger: batch insert failed", "count", len(batch), "error", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case event := <-l.ch:
			batch = append(batch, event)
			if len(batch) >= auditBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			for {
				select {
				case event := <-l.ch:
					batch = append(batch, event)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (l *auditLogger) Done() <-chan struct{} {
	return l.done
}
