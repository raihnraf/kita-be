package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"kita-be/internal/platform/logger"
)

type MessageHandler func(msg Message) error

type Message struct {
	EventID                  string `json:"event_id"`
	EventType                string `json:"event_type"`
	Result                   string `json:"result,omitempty"`
	ErrorMessage             string `json:"error_message,omitempty"`
	TransactionID            string `json:"transaction_id"`
	TransactionRef           string `json:"transaction_ref"`
	CompensationForEventType string `json:"compensation_for_event_type,omitempty"`
	CompensationReason       string `json:"compensation_reason,omitempty"`
	UserID                   string `json:"user_id"`
	BookID                   string `json:"book_id"`
	Quantity                 int    `json:"quantity"`
	OccurredAt               string `json:"occurred_at"`
	IdempotencyKey           string `json:"idempotency_key"`
	RetryCount               int32  `json:"-"`
	DeliveryTag              uint64 `json:"-"`
}

type Consumer struct {
	conn      *Connection
	queueName string
}

func NewConsumer(conn *Connection, queueName string) *Consumer {
	return &Consumer{conn: conn, queueName: queueName}
}

// Setup declares the exchange, main queue (with DLQ routing), and the DLQ.
// Both the publisher and consumer call this so topology is always consistent.
func (c *Consumer) Setup() error {
	if !c.conn.IsConnected() {
		return nil
	}

	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	return declareTopology(ch)
}

// ConsumeWithReconnect runs Consume in a loop, reconnecting on broker disconnects.
// It returns only when ctx is cancelled or a permanent error occurs.
func (c *Consumer) ConsumeWithReconnect(ctx context.Context, handler MessageHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !c.conn.IsConnected() {
			if err := c.conn.Reconnect(30, 2*time.Second); err != nil {
				logger.Error("rabbitmq reconnect exhausted, consumer stopping", "error", err.Error())
				return
			}
			if err := c.Setup(); err != nil {
				logger.Error("rabbitmq topology re-setup failed", "error", err.Error())
				return
			}
		}

		if err := c.consume(ctx, handler); err != nil {
			logger.Warn("rabbitmq consumer loop exited, will reconnect", "error", err.Error())
			// small pause before reconnect
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			// mark connection as closed so next iteration reconnects
			c.conn.Close()
		}
	}
}

// Consume runs one consume session. Returns when the deliveries channel closes.
func (c *Consumer) Consume(handler MessageHandler) error {
	return c.consume(context.Background(), handler)
}

func (c *Consumer) consume(ctx context.Context, handler MessageHandler) error {
	if !c.conn.IsConnected() {
		return fmt.Errorf("rabbitmq not connected")
	}

	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("failed to set qos: %w", err)
	}

	deliveries, err := ch.Consume(
		c.queueName, "", false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	logger.Info("rabbitmq consumer started", "queue", c.queueName)

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("deliveries channel closed (broker disconnect?)")
			}
			c.processDelivery(ch, delivery, handler)
		}
	}
}

func (c *Consumer) processDelivery(ch *amqp.Channel, delivery amqp.Delivery, handler MessageHandler) {
	var msg Message
	if err := json.Unmarshal(delivery.Body, &msg); err != nil {
		logger.Error("failed to unmarshal message, sending to DLQ", "error", err.Error())
		// bad payload → nack without requeue → goes to DLQ
		if nackErr := delivery.Nack(false, false); nackErr != nil {
			logger.Error("failed to nack malformed message", "error", nackErr.Error())
		}
		return
	}

	msg.DeliveryTag = delivery.DeliveryTag

	if retryCount, ok := delivery.Headers["x-retry-count"].(int32); ok {
		msg.RetryCount = retryCount
	}

	if err := handler(msg); err != nil {
		logger.Warn("message processing failed",
			"event_id", msg.EventID,
			"event_type", msg.EventType,
			"retry_count", msg.RetryCount,
			"error", err.Error(),
		)

		const maxRetries = 3
		if msg.RetryCount < maxRetries {
			if retryErr := c.retryPublish(ch, msg, delivery.RoutingKey); retryErr != nil {
				logger.Error("failed to re-publish for retry, sending to DLQ",
					"event_id", msg.EventID,
					"error", retryErr.Error(),
				)
				// fall through to nack → DLQ
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					logger.Error("failed to nack message after retry publish failure", "error", nackErr.Error())
				}
				return
			}
			// ack the original; the retry copy is already enqueued
			if ackErr := delivery.Ack(false); ackErr != nil {
				logger.Error("failed to ack message after retry publish", "error", ackErr.Error())
			}
		} else {
			logger.Error("message exceeded max retries, sending to DLQ",
				"event_id", msg.EventID,
				"retry_count", msg.RetryCount,
			)
			if nackErr := delivery.Nack(false, false); nackErr != nil {
				logger.Error("failed to nack exhausted message", "error", nackErr.Error())
			}
		}
		return
	}

	if ackErr := delivery.Ack(false); ackErr != nil {
		logger.Error("failed to ack message", "event_id", msg.EventID, "error", ackErr.Error())
	}
}

// retryPublish publishes a retry copy of the message using a fresh channel so it
// doesn't interfere with the consuming channel.
func (c *Consumer) retryPublish(ch *amqp.Channel, msg Message, routingKey string) error {
	msg.RetryCount++
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal retry message: %w", err)
	}

	headers := amqp.Table{"x-retry-count": msg.RetryCount}

	// Use the existing channel (same connection, safe for publish while consuming
	// on a separate channel via Qos=1). A new channel would require a connection
	// check; reusing avoids the extra round-trip while still being safe.
	if err := ch.PublishWithContext(
		context.Background(),
		ExchangeName, routingKey,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Headers:      headers,
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return fmt.Errorf("failed to publish retry: %w", err)
	}

	logger.Info("message re-queued for retry",
		"event_id", msg.EventID,
		"retry_count", msg.RetryCount,
	)
	return nil
}
