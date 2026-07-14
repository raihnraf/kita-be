package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "kita-be/internal/transaction/domain"
)

func TestOutboxDispatcherMarksPublishedOnSuccess(t *testing.T) {
	repo := &fakeOutboxRepo{
		events: []domain.StockEventOutbox{{ID: "event-1", EventType: "DECREASE", Attempts: 1}},
	}
	publisher := &fakeOutboxPublisher{}
	dispatcher := NewOutboxDispatcher(repo, publisher, time.Second, 10)

	dispatcher.dispatchOnce(context.Background())

	if len(publisher.published) != 1 || publisher.published[0] != "event-1" {
		t.Fatalf("expected event published once, got %+v", publisher.published)
	}
	if len(repo.published) != 1 || repo.published[0] != "event-1" {
		t.Fatalf("expected event marked published, got %+v", repo.published)
	}
	if len(repo.failed) != 0 {
		t.Fatalf("expected no failed marks, got %+v", repo.failed)
	}
}

func TestOutboxDispatcherMarksFailedOnPublishError(t *testing.T) {
	repo := &fakeOutboxRepo{
		events: []domain.StockEventOutbox{{ID: "event-1", EventType: "INCREASE", Attempts: 2}},
	}
	publisher := &fakeOutboxPublisher{err: errors.New("broker unavailable")}
	dispatcher := NewOutboxDispatcher(repo, publisher, time.Second, 10)

	dispatcher.dispatchOnce(context.Background())

	if len(repo.published) != 0 {
		t.Fatalf("expected no published marks, got %+v", repo.published)
	}
	if len(repo.failed) != 1 || repo.failed[0] != "event-1" {
		t.Fatalf("expected event marked failed, got %+v", repo.failed)
	}
	if repo.nextAttemptAt.IsZero() || !repo.nextAttemptAt.After(time.Now().UTC()) {
		t.Fatalf("expected future retry time, got %s", repo.nextAttemptAt)
	}
}

type fakeOutboxRepo struct {
	events        []domain.StockEventOutbox
	published     []string
	failed        []string
	nextAttemptAt time.Time
}

func (r *fakeOutboxRepo) ClaimDue(ctx context.Context, limit int) ([]domain.StockEventOutbox, error) {
	return r.events, nil
}

func (r *fakeOutboxRepo) MarkPublished(ctx context.Context, id string) error {
	r.published = append(r.published, id)
	return nil
}

func (r *fakeOutboxRepo) MarkFailed(ctx context.Context, id string, publishErr error, nextAttemptAt time.Time) error {
	r.failed = append(r.failed, id)
	r.nextAttemptAt = nextAttemptAt
	return nil
}

type fakeOutboxPublisher struct {
	published []string
	err       error
}

func (p *fakeOutboxPublisher) PublishStockEvent(ctx context.Context, event domain.StockEventOutbox) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event.ID)
	return nil
}
