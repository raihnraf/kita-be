package messaging

import (
	"context"
	"time"

	"github.com/google/uuid"

	"kita-be/internal/platform/rabbitmq"
)

type StockEventPayload struct {
	EventID        string `json:"event_id"`
	EventType      string `json:"event_type"`
	TransactionID  string `json:"transaction_id"`
	TransactionRef string `json:"transaction_ref"`
	UserID         string `json:"user_id"`
	BookID         string `json:"book_id"`
	Quantity       int    `json:"quantity"`
	OccurredAt     string `json:"occurred_at"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Publisher struct {
	rmq *rabbitmq.Publisher
}

func NewPublisher(rmq *rabbitmq.Publisher) *Publisher {
	return &Publisher{rmq: rmq}
}

func (p *Publisher) PublishStockDecrease(ctx context.Context, transactionID, transactionRef, userID, bookID string) error {
	return p.publish(ctx, "DECREASE", rabbitmq.RoutingKeyDec, transactionID, transactionRef, userID, bookID)
}

func (p *Publisher) PublishStockIncrease(ctx context.Context, transactionID, transactionRef, userID, bookID string) error {
	return p.publish(ctx, "INCREASE", rabbitmq.RoutingKeyInc, transactionID, transactionRef, userID, bookID)
}

func (p *Publisher) publish(ctx context.Context, eventType, routingKey, transactionID, transactionRef, userID, bookID string) error {
	payload := StockEventPayload{
		EventID:        uuid.New().String(),
		EventType:      eventType,
		TransactionID:  transactionID,
		TransactionRef: transactionRef,
		UserID:         userID,
		BookID:         bookID,
		Quantity:       1,
		OccurredAt:     time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey: uuid.New().String(),
	}

	return p.rmq.Publish(ctx, routingKey, payload)
}
