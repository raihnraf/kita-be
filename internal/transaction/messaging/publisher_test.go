package messaging

import (
	"testing"
	"time"

	"kita-be/internal/platform/rabbitmq"
	domain "kita-be/internal/transaction/domain"
)

func TestStockEventPayloadFromOutboxIncludesCompensationMetadata(t *testing.T) {
	compensates := "DECREASE"
	reason := "borrow_create_failed"
	occurredAt := time.Date(2026, 7, 14, 4, 5, 6, 0, time.UTC)
	event := domain.StockEventOutbox{
		ID:                       "evt-1",
		EventType:                "INCREASE",
		TransactionID:            "txn-1",
		TransactionRef:           "TXN-1",
		CompensationForEventType: &compensates,
		CompensationReason:       &reason,
		UserID:                   "user-1",
		BookID:                   "book-1",
		Quantity:                 1,
		CreatedAt:                occurredAt,
	}

	resolvedPayload, err := stockEventPayloadFromOutbox(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolvedPayload.EventID != "evt-1" || resolvedPayload.EventType != rabbitmq.EventTypeIncreaseStockRequested {
		t.Fatalf("unexpected event identity in payload: %+v", resolvedPayload)
	}
	if resolvedPayload.TransactionID != "txn-1" || resolvedPayload.TransactionRef != "TXN-1" {
		t.Fatalf("unexpected transaction linkage in payload: %+v", resolvedPayload)
	}
	if resolvedPayload.CompensationForEventType == nil || *resolvedPayload.CompensationForEventType != rabbitmq.EventTypeDecreaseStockRequested {
		t.Fatalf("expected compensation_for_event_type=%s, got %+v", rabbitmq.EventTypeDecreaseStockRequested, resolvedPayload.CompensationForEventType)
	}
	if resolvedPayload.CompensationReason == nil || *resolvedPayload.CompensationReason != reason {
		t.Fatalf("expected compensation_reason=%s, got %+v", reason, resolvedPayload.CompensationReason)
	}
	if resolvedPayload.OccurredAt != occurredAt.Format(time.RFC3339) {
		t.Fatalf("expected occurred_at=%s, got %s", occurredAt.Format(time.RFC3339), resolvedPayload.OccurredAt)
	}
	if resolvedPayload.IdempotencyKey != "evt-1" {
		t.Fatalf("expected idempotency_key to mirror event id, got %s", resolvedPayload.IdempotencyKey)
	}
}

func TestStockEventPayloadFromOutboxOmitsCompensationMetadataForNormalEvents(t *testing.T) {
	event := domain.StockEventOutbox{
		ID:             "evt-2",
		EventType:      "DECREASE",
		TransactionID:  "txn-2",
		TransactionRef: "TXN-2",
		UserID:         "user-2",
		BookID:         "book-2",
		Quantity:       1,
		CreatedAt:      time.Date(2026, 7, 14, 4, 5, 6, 0, time.UTC),
	}

	payload, err := stockEventPayloadFromOutbox(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.CompensationForEventType != nil || payload.CompensationReason != nil {
		t.Fatalf("expected normal stock event payload to omit compensation metadata, got %+v", payload)
	}
	if payload.EventType != rabbitmq.EventTypeDecreaseStockRequested {
		t.Fatalf("expected event type %s, got %s", rabbitmq.EventTypeDecreaseStockRequested, payload.EventType)
	}
}
