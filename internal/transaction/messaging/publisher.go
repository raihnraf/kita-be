package messaging

import (
	"context"
	"time"

	"kita-be/internal/platform/rabbitmq"
	domain "kita-be/internal/transaction/domain"
)

type StockEventPayload struct {
	EventID                  string  `json:"event_id"`
	EventType                string  `json:"event_type"`
	TransactionID            string  `json:"transaction_id"`
	TransactionRef           string  `json:"transaction_ref"`
	CompensationForEventType *string `json:"compensation_for_event_type,omitempty"`
	CompensationReason       *string `json:"compensation_reason,omitempty"`
	UserID                   string  `json:"user_id"`
	BookID                   string  `json:"book_id"`
	Quantity                 int     `json:"quantity"`
	OccurredAt               string  `json:"occurred_at"`
	IdempotencyKey           string  `json:"idempotency_key"`
}

type Publisher struct {
	rmq *rabbitmq.Publisher
}

func NewPublisher(rmq *rabbitmq.Publisher) *Publisher {
	return &Publisher{rmq: rmq}
}

func (p *Publisher) PublishStockEvent(ctx context.Context, event domain.StockEventOutbox) error {
	routingKey := rabbitmq.RoutingKeyDec
	if event.EventType == "INCREASE" {
		routingKey = rabbitmq.RoutingKeyInc
	}

	payload := stockEventPayloadFromOutbox(event)

	return p.rmq.Publish(ctx, routingKey, payload)
}

func stockEventPayloadFromOutbox(event domain.StockEventOutbox) StockEventPayload {
	return StockEventPayload{
		EventID:                  event.ID,
		EventType:                event.EventType,
		TransactionID:            event.TransactionID,
		TransactionRef:           event.TransactionRef,
		CompensationForEventType: event.CompensationForEventType,
		CompensationReason:       event.CompensationReason,
		UserID:                   event.UserID,
		BookID:                   event.BookID,
		Quantity:                 event.Quantity,
		OccurredAt:               event.CreatedAt.UTC().Format(time.RFC3339),
		IdempotencyKey:           event.ID,
	}
}
