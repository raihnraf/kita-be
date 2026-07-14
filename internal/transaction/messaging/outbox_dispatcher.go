package messaging

import (
	"context"
	"time"

	"kita-be/internal/platform/logger"
	domain "kita-be/internal/transaction/domain"
)

type StockEventOutboxRepository interface {
	ClaimDue(ctx context.Context, limit int) ([]domain.StockEventOutbox, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, publishErr error, nextAttemptAt time.Time) error
}

type StockEventPublisher interface {
	PublishStockEvent(ctx context.Context, event domain.StockEventOutbox) error
}

type OutboxDispatcher struct {
	repo      StockEventOutboxRepository
	publisher StockEventPublisher
	interval  time.Duration
	batchSize int
}

func NewOutboxDispatcher(repo StockEventOutboxRepository, publisher StockEventPublisher, interval time.Duration, batchSize int) *OutboxDispatcher {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 10
	}
	return &OutboxDispatcher{repo: repo, publisher: publisher, interval: interval, batchSize: batchSize}
}

func (d *OutboxDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		d.dispatchOnce(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *OutboxDispatcher) dispatchOnce(ctx context.Context) {
	events, err := d.repo.ClaimDue(ctx, d.batchSize)
	if err != nil {
		logger.Warn("stock outbox claim failed", "error", err.Error())
		return
	}

	for _, event := range events {
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := d.publisher.PublishStockEvent(publishCtx, event)
		cancel()

		if err != nil {
			nextAttemptAt := time.Now().UTC().Add(backoff(event.Attempts))
			if markErr := d.repo.MarkFailed(context.Background(), event.ID, err, nextAttemptAt); markErr != nil {
				logger.Error("stock outbox mark failed status failed", "event_id", event.ID, "error", markErr.Error())
			}
			logger.Warn("stock outbox publish failed", "event_id", event.ID, "attempts", event.Attempts, "error", err.Error())
			continue
		}

		if err := d.repo.MarkPublished(context.Background(), event.ID); err != nil {
			logger.Error("stock outbox mark published failed", "event_id", event.ID, "error", err.Error())
			continue
		}
		logger.Info("stock outbox published", "event_id", event.ID, "event_type", event.EventType)
	}
}

func backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(attempts*attempts) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
