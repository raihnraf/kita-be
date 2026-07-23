package rabbitmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"kita-be/internal/platform/logger"
)

const (
	ExchangeName        = "library.events"
	ExchangeType        = "topic"
	CommandQueueName    = "book.stock.commands"
	CommandDLQName      = "book.stock.commands.dlq"
	ResultQueueName     = "transaction.stock.results"
	ResultDLQName       = "transaction.stock.results.dlq"
	QueueName           = CommandQueueName
	DLQName             = CommandDLQName
	RoutingKeyDec       = "stock.decrease.requested"
	RoutingKeyInc       = "stock.increase.requested"
	RoutingKeyDecResult = "stock.decrease.result"
	RoutingKeyIncResult = "stock.increase.result"
	ResultSucceeded     = "SUCCEEDED"
	ResultRejected      = "REJECTED"
)

// Connection wraps an AMQP connection with reconnect capability.
type Connection struct {
	conn *amqp.Connection
	url  string
}

func NewConnection(url string) (*Connection, error) {
	if url == "" {
		return &Connection{}, nil
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	return &Connection{conn: conn, url: url}, nil
}

// Reconnect attempts to re-establish the AMQP connection with exponential backoff.
// It blocks until a connection is established or maxAttempts is exhausted.
func (c *Connection) Reconnect(maxAttempts int, initialDelay time.Duration) error {
	if c.url == "" {
		return fmt.Errorf("rabbitmq url is empty, cannot reconnect")
	}

	delay := initialDelay
	for i := 1; i <= maxAttempts; i++ {
		logger.Warn("rabbitmq reconnecting", "attempt", i, "max_attempts", maxAttempts)
		conn, err := amqp.Dial(c.url)
		if err == nil {
			c.conn = conn
			logger.Info("rabbitmq reconnected successfully")
			return nil
		}
		logger.Warn("rabbitmq reconnect attempt failed", "attempt", i, "error", err.Error())
		if i < maxAttempts {
			time.Sleep(delay)
			if delay < 30*time.Second {
				delay *= 2
			}
		}
	}
	return fmt.Errorf("rabbitmq reconnect failed after %d attempts", maxAttempts)
}

func (c *Connection) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Connection) IsConnected() bool {
	return c.conn != nil && !c.conn.IsClosed()
}

func (c *Connection) Channel() (*amqp.Channel, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("rabbitmq not connected")
	}
	return c.conn.Channel()
}

func (c *Connection) GetConn() *amqp.Connection {
	return c.conn
}

// ConnectWithRetry attempts to establish a connection to RabbitMQ with retry and exponential backoff.
func ConnectWithRetry(url string, attempts int, initialDelay time.Duration) (*Connection, error) {
	if url == "" {
		return nil, fmt.Errorf("rabbitmq url is empty")
	}

	var lastErr error
	delay := initialDelay
	for i := 1; i <= attempts; i++ {
		conn, err := NewConnection(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		logger.Warn("rabbitmq connection attempt failed", "attempt", i, "max_attempts", attempts, "error", err.Error())
		if i < attempts {
			time.Sleep(delay)
			if delay < 30*time.Second {
				delay *= 2
			}
		}
	}
	return nil, fmt.Errorf("rabbitmq connection failed after %d attempts: %w", attempts, lastErr)
}
