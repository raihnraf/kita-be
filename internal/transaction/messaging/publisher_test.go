package messaging

import (
	"testing"
	"time"

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

	payload := stockEventPayloadFromOutbox(event)

	if payload.EventID != "evt-1" || payload.EventType != "INCREASE" {
		t.Fatalf("unexpected event identity in payload: %+v", payload)
	}
	if payload.TransactionID != "txn-1" || payload.TransactionRef != "TXN-1" {
		t.Fatalf("unexpected transaction linkage in payload: %+v", payload)
	}
	if payload.CompensationForEventType == nil || *payload.CompensationForEventType != compensates {
		t.Fatalf("expected compensation_for_event_type=%s, got %+v", compensates, payload.CompensationForEventType)
	}
	if payload.CompensationReason == nil || *payload.CompensationReason != reason {
		t.Fatalf("expected compensation_reason=%s, got %+v", reason, payload.CompensationReason)
	}
	if payload.OccurredAt != occurredAt.Format(time.RFC3339) {
		t.Fatalf("expected occurred_at=%s, got %s", occurredAt.Format(time.RFC3339), payload.OccurredAt)
	}
	if payload.IdempotencyKey != "evt-1" {
		t.Fatalf("expected idempotency_key to mirror event id, got %s", payload.IdempotencyKey)
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

	payload := stockEventPayloadFromOutbox(event)

	if payload.CompensationForEventType != nil || payload.CompensationReason != nil {
		t.Fatalf("expected normal stock event payload to omit compensation metadata, got %+v", payload)
	}
}
