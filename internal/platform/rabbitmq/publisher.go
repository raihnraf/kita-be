package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"kita-be/internal/platform/logger"
)

type Publisher struct {
	conn *Connection
}

func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Setup declares exchange + queues using the shared topology helpers.
// Topology is identical to Consumer.Setup() so both sides always agree.
func (p *Publisher) Setup() error {
	if !p.conn.IsConnected() {
		return nil
	}

	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.ExchangeDeclare(
		ExchangeName, ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	queueArgs := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": DLQName,
	}
	if _, err := ch.QueueDeclare(QueueName, true, false, false, false, queueArgs); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", QueueName, err)
	}

	for _, rk := range []string{RoutingKeyDec, RoutingKeyInc} {
		if err := ch.QueueBind(QueueName, rk, ExchangeName, false, nil); err != nil {
			return fmt.Errorf("failed to bind queue (routing_key=%s): %w", rk, err)
		}
	}

	if _, err := ch.QueueDeclare(DLQName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare dlq %s: %w", DLQName, err)
	}

	return nil
}

// Publish sends a message to the exchange using publisher confirms.
// Returns an error if the broker nacks the message or the context expires.
// Logs a warning and returns nil (non-fatal) when not connected so callers
// can decide whether to treat async publish failure as critical.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	if !p.conn.IsConnected() {
		logger.Warn("rabbitmq not connected, skipping publish", "routing_key", routingKey)
		return nil
	}

	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	if err := ch.PublishWithContext(ctx,
		ExchangeName, routingKey,
		true, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	select {
	case conf := <-confirms:
		if !conf.Ack {
			return fmt.Errorf("message nacked by broker (routing_key=%s)", routingKey)
		}
		logger.Info("message published and confirmed", "routing_key", routingKey)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("publish confirm timed out: %w", ctx.Err())
	}
}
