package messaging

import (
	"context"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/rabbitmq"
)

type ResultPublisher interface {
	PublishStockResult(ctx context.Context, event *domain.BookStockEvent) error
}

type Publisher struct {
	rmq *rabbitmq.Publisher
}

func NewPublisher(rmq *rabbitmq.Publisher) *Publisher {
	return &Publisher{rmq: rmq}
}

func (p *Publisher) PublishStockResult(ctx context.Context, event *domain.BookStockEvent) error {
	rejected := event.Status == domain.StockEventFailed
	eventType, err := rabbitmq.ResultEventTypeForOperation(string(event.EventType), rejected)
	if err != nil {
		return err
	}
	routingKey, err := rabbitmq.RoutingKeyForResultEventType(eventType)
	if err != nil {
		return err
	}

	message := ""
	if rejected && event.ErrorMessage != nil {
		message = *event.ErrorMessage
	}

	payload := rabbitmq.Message{
		EventID:        event.EventID,
		EventType:      eventType,
		ErrorMessage:   message,
		TransactionID:  event.TransactionID,
		BookID:         event.BookID,
		Quantity:       event.Quantity,
		IdempotencyKey: event.EventID,
	}

	return p.rmq.Publish(ctx, routingKey, payload)
}
