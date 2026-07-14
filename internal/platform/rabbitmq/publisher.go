package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"kita-be/internal/platform/logger"
)

type queueBinding struct {
	queueName string
	dlqName   string
	bindings  []string
}

func topologyBindings() []queueBinding {
	return []queueBinding{
		{
			queueName: CommandQueueName,
			dlqName:   CommandDLQName,
			bindings:  []string{RoutingKeyDec, RoutingKeyInc},
		},
		{
			queueName: ResultQueueName,
			dlqName:   ResultDLQName,
			bindings:  []string{RoutingKeyDecResult, RoutingKeyIncResult},
		},
	}
}

func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		ExchangeName, ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	for _, topology := range topologyBindings() {
		queueArgs := amqp.Table{
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": topology.dlqName,
		}
		if _, err := ch.QueueDeclare(topology.queueName, true, false, false, false, queueArgs); err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", topology.queueName, err)
		}
		for _, routingKey := range topology.bindings {
			if err := ch.QueueBind(topology.queueName, routingKey, ExchangeName, false, nil); err != nil {
				return fmt.Errorf("failed to bind queue %s (routing_key=%s): %w", topology.queueName, routingKey, err)
			}
		}
		if _, err := ch.QueueDeclare(topology.dlqName, true, false, false, false, nil); err != nil {
			return fmt.Errorf("failed to declare dlq %s: %w", topology.dlqName, err)
		}
	}

	return nil
}

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

	return declareTopology(ch)
}

// Publish sends a message to the exchange using publisher confirms.
// Returns an error if the broker nacks the message or the context expires.
// Attempts a reconnect when disconnected and returns an error if publishing
// cannot be confirmed, allowing durable outbox dispatchers to retry later.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	if !p.conn.IsConnected() {
		logger.Warn("rabbitmq not connected, attempting reconnect", "routing_key", routingKey)
		if err := p.conn.Reconnect(3, time.Second); err != nil {
			return fmt.Errorf("rabbitmq not connected: %w", err)
		}
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
